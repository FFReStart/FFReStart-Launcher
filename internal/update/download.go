package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const maxArtifactSize int64 = 256 << 20

var (
	ErrDownloadTruncated = errors.New("update download is truncated")
	ErrDownloadHash      = errors.New("update download SHA-256 rejected")
)

type progressRecord struct {
	URL        string `json:"url"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	Downloaded int64  `json:"downloaded"`
}

// Downloader persists partial bytes and their signed identity beside the
// destination. A retry resumes only when both records agree with the manifest.
type Downloader struct{ Client *http.Client }

func (d Downloader) Download(ctx context.Context, artifactURL, destination string, size int64, expectedHash string) error {
	if size <= 0 || size > maxArtifactSize {
		return ErrInvalidSize
	}
	if decoded, err := hex.DecodeString(expectedHash); err != nil || len(decoded) != sha256.Size {
		return ErrDownloadHash
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	for attempt := 0; attempt < 2; attempt++ {
		err := d.downloadOnce(ctx, artifactURL, destination, size, strings.ToLower(expectedHash))
		if !errors.Is(err, ErrDownloadHash) || attempt == 1 {
			return err
		}
		if resetErr := resetPartial(destination); resetErr != nil {
			return errors.Join(err, resetErr)
		}
	}
	return ErrDownloadHash
}

func (d Downloader) downloadOnce(ctx context.Context, artifactURL, destination string, size int64, expectedHash string) error {
	partPath := destination + ".part"
	progressPath := destination + ".progress.json"
	downloaded, err := reusableProgress(partPath, progressPath, progressRecord{URL: artifactURL, Size: size, SHA256: expectedHash})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return err
	}
	if downloaded > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
	}
	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if downloaded > 0 && response.StatusCode == http.StatusOK {
		downloaded = 0
		if err := resetPartial(destination); err != nil {
			return err
		}
	}
	if downloaded == 0 && response.StatusCode != http.StatusOK || downloaded > 0 && response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("update download HTTP %d", response.StatusCode)
	}
	if downloaded > 0 && !validContentRange(response.Header.Get("Content-Range"), downloaded, size) {
		return errors.New("update download returned an invalid content range")
	}
	flags := os.O_CREATE | os.O_WRONLY
	if downloaded > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	part, err := os.OpenFile(partPath, flags, 0o600) // #nosec G304 -- path is confined to the application-controlled destination.
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(part, io.LimitReader(response.Body, size-downloaded+1))
	closeErr := part.Close()
	downloaded += written
	if err := writeProgress(progressPath, progressRecord{URL: artifactURL, Size: size, SHA256: expectedHash, Downloaded: downloaded}); err != nil {
		return errors.Join(copyErr, closeErr, err)
	}
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	if downloaded < size {
		return ErrDownloadTruncated
	}
	if downloaded > size {
		_ = resetPartial(destination)
		return ErrInvalidSize
	}
	if err := verifyFile(partPath, size, expectedHash); err != nil {
		return err
	}
	if err := atomicReplace(partPath, destination); err != nil {
		return err
	}
	return os.Remove(progressPath)
}

func reusableProgress(partPath, progressPath string, expected progressRecord) (int64, error) {
	data, readErr := os.ReadFile(progressPath) // #nosec G304 -- caller controls app-data paths.
	info, statErr := os.Stat(partPath)
	if os.IsNotExist(readErr) && os.IsNotExist(statErr) {
		return 0, nil
	}
	var progress progressRecord
	valid := readErr == nil && statErr == nil && json.Unmarshal(data, &progress) == nil &&
		progress.URL == expected.URL && progress.Size == expected.Size && strings.EqualFold(progress.SHA256, expected.SHA256) &&
		progress.Downloaded == info.Size() && info.Size() >= 0 && info.Size() < expected.Size
	if !valid {
		if err := resetPartial(strings.TrimSuffix(partPath, ".part")); err != nil {
			return 0, err
		}
		return 0, nil
	}
	return info.Size(), nil
}

func validContentRange(value string, start, size int64) bool {
	return value == fmt.Sprintf("bytes %d-%d/%d", start, size-1, size)
}

func writeProgress(path string, progress progressRecord) error {
	data, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	return AtomicWriteFile(path, data, 0o600)
}

// AtomicWriteFile replaces a small control file without exposing partial
// contents to a concurrent launcher process.
func AtomicWriteFile(path string, data []byte, permission os.FileMode) error {
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, permission) // #nosec G304 -- caller controls app-data paths.
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return atomicReplace(temporary, path)
}

func resetPartial(destination string) error {
	var errs []error
	for _, path := range []string{destination + ".part", destination + ".progress.json", destination + ".progress.json.tmp"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func verifyFile(path string, size int64, expectedHash string) error {
	file, err := os.Open(path) // #nosec G304 -- caller controls app-data paths.
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	if written != size {
		return ErrDownloadTruncated
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expectedHash) {
		return ErrDownloadHash
	}
	return nil
}
