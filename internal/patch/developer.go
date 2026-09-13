package patch

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/FFReStart/FFReStart-Launcher/internal/update"
)

// The developer channel intentionally matches the original WPF launcher. It
// is unsigned and must never be used when a signed manifest channel exists.
const (
	DeveloperVersionURL = "https://github.com/FFReStart/FFReStart_Builds/releases/latest/download/version.txt"
	DeveloperArchiveURL = "https://github.com/FFReStart/FFReStart_Builds/releases/latest/download/FFReStart-Dev-Build.zip"
	developerBuildDir   = "FFReStart-Dev-Build"
	developerGameExe    = "FFReStart-Dev-Build.exe"
	developerVersion    = "Version.txt"
	maxDeveloperArchive = int64(2 << 30)
	maxDeveloperFile    = int64(1 << 30)
	maxDeveloperTotal   = int64(4 << 30)
	maxDeveloperEntries = 100000
)

type DeveloperInstaller struct {
	Root     string
	Client   *http.Client
	Progress func(received, total int64)
}

func (i DeveloperInstaller) InstallOrUpdate(ctx context.Context) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, errors.New("the unsigned developer build channel is Windows-only")
	}
	versionData, err := i.fetch(ctx, DeveloperVersionURL, 64)
	if err != nil {
		return false, fmt.Errorf("read developer build version: %w", err)
	}
	online := parseDeveloperVersion(strings.TrimSpace(string(versionData)))
	if online == (developerVersionValue{}) {
		return false, errors.New("developer build returned an invalid version")
	}
	localData, _ := os.ReadFile(filepath.Join(i.Root, developerVersion)) // #nosec G304 -- user-selected install root.
	local := parseDeveloperVersion(strings.TrimSpace(string(localData)))
	installed := usableDeveloperExecutable(filepath.Join(i.Root, developerBuildDir, developerGameExe))
	if installed && local == online {
		return false, nil
	}
	if err := os.MkdirAll(i.Root, 0o700); err != nil {
		return false, err
	}
	archive, err := os.CreateTemp(i.Root, ".ffrestart-developer-*.zip")
	if err != nil {
		return false, err
	}
	archivePath := archive.Name()
	if err := archive.Close(); err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(archivePath) }()
	if err := i.download(ctx, DeveloperArchiveURL, archivePath); err != nil {
		return false, fmt.Errorf("download developer build: %w", err)
	}
	staging, err := os.MkdirTemp(i.Root, ".ffrestart-developer-staging-*")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	if err := extractDeveloperZip(archivePath, staging); err != nil {
		return false, err
	}
	stagedBuild := filepath.Join(staging, developerBuildDir)
	if !usableDeveloperExecutable(filepath.Join(stagedBuild, developerGameExe)) {
		return false, errors.New("developer build archive omitted the expected executable")
	}
	if err := promoteDeveloperBuild(i.Root, stagedBuild, strings.TrimSpace(string(versionData))); err != nil {
		return false, err
	}
	return true, nil
}

func (i DeveloperInstaller) fetch(ctx context.Context, target string, limit int64) ([]byte, error) {
	parsed, parseErr := url.Parse(target)
	if parseErr != nil || !allowedDeveloperURL(parsed, true) {
		return nil, errors.New("developer channel URL rejected")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	response, err := i.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("response exceeds size limit")
	}
	return data, nil
}

func (i DeveloperInstaller) download(ctx context.Context, target, destination string) error {
	parsed, parseErr := url.Parse(target)
	if parseErr != nil || !allowedDeveloperURL(parsed, true) {
		return errors.New("developer channel URL rejected")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	response, err := i.httpClient().Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxDeveloperArchive {
		return errors.New("archive exceeds size limit")
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- generated file in selected root.
	if err != nil {
		return err
	}
	reader := &developerProgressReader{reader: io.LimitReader(response.Body, maxDeveloperArchive+1), total: response.ContentLength, report: i.Progress}
	written, copyErr := io.Copy(output, reader)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(copyErr, closeErr)
	}
	if written > maxDeveloperArchive {
		return errors.New("archive exceeds size limit")
	}
	return nil
}

func (i DeveloperInstaller) httpClient() *http.Client {
	base := i.Client
	if base == nil {
		base = http.DefaultClient
	}
	copy := *base
	copy.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) > 10 || !allowedDeveloperURL(request.URL, false) {
			return errors.New("developer channel redirect rejected")
		}
		return nil
	}
	return &copy
}

func allowedDeveloperURL(value *url.URL, initial bool) bool {
	if value == nil || value.Scheme != "https" || value.User != nil || (value.Port() != "" && value.Port() != "443") {
		return false
	}
	host := strings.ToLower(value.Hostname())
	if initial {
		return host == "github.com" && value.RawQuery == "" && value.Fragment == "" && (value.EscapedPath() == "/FFReStart/FFReStart_Builds/releases/latest/download/version.txt" || value.EscapedPath() == "/FFReStart/FFReStart_Builds/releases/latest/download/FFReStart-Dev-Build.zip")
	}
	switch host {
	case "github.com":
		const releasePrefix = "/FFReStart/FFReStart_Builds/releases/download/"
		remainder := strings.TrimPrefix(value.EscapedPath(), releasePrefix)
		parts := strings.Split(remainder, "/")
		return remainder != value.EscapedPath() && len(parts) == 2 && parts[0] != "" && (parts[1] == "version.txt" || parts[1] == "FFReStart-Dev-Build.zip")
	case "release-assets.githubusercontent.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
		return true
	}
	return false
}

type developerProgressReader struct {
	reader          io.Reader
	received, total int64
	report          func(int64, int64)
}

func (r *developerProgressReader) Read(buffer []byte) (int, error) {
	count, err := r.reader.Read(buffer)
	r.received += int64(count)
	if r.report != nil {
		r.report(r.received, r.total)
	}
	return count, err
}

func extractDeveloperZip(archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open developer build archive: %w", err)
	}
	defer func() { _ = archive.Close() }()
	if len(archive.File) == 0 || len(archive.File) > maxDeveloperEntries {
		return errors.New("developer build archive has an invalid entry count")
	}
	var total int64
	for _, entry := range archive.File {
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 || entry.UncompressedSize64 > uint64(maxDeveloperFile) {
			return errors.New("developer build archive contains an unsafe entry")
		}
		if entry.UncompressedSize64 > uint64(maxDeveloperTotal)-uint64(total) {
			return errors.New("developer build archive exceeds extraction limit")
		}
		total += int64(entry.UncompressedSize64)
		clean := filepath.Clean(filepath.FromSlash(entry.Name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return errors.New("developer build archive contains path traversal")
		}
		target := filepath.Join(destination, clean)
		relative, relErr := filepath.Rel(destination, target)
		if relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return errors.New("developer build archive escapes staging")
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- zip-safe staging target.
		if err != nil {
			_ = input.Close()
			return err
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, maxDeveloperFile+1))
		closeErr := errors.Join(output.Close(), input.Close())
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		if written != int64(entry.UncompressedSize64) {
			return errors.New("developer build archive entry size mismatch")
		}
	}
	return nil
}

func promoteDeveloperBuild(root, stagedBuild, version string) error {
	target := filepath.Join(root, developerBuildDir)
	backup := target + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove previous developer backup: %w", err)
	}
	hadCurrent := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("retain current developer build: %w", err)
		}
		hadCurrent = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stagedBuild, target); err != nil {
		if hadCurrent {
			_ = os.Rename(backup, target)
		}
		return fmt.Errorf("promote developer build: %w", err)
	}
	if err := update.AtomicWriteFile(filepath.Join(root, developerVersion), []byte(version), 0o600); err != nil {
		_ = os.RemoveAll(target)
		if hadCurrent {
			_ = os.Rename(backup, target)
		}
		return fmt.Errorf("record developer version: %w", err)
	}
	return nil
}

func usableDeveloperExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

type developerVersionValue struct{ major, minor, patch int16 }

func parseDeveloperVersion(value string) developerVersionValue {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return developerVersionValue{}
	}
	numbers := make([]int16, 3)
	for index, part := range parts {
		parsed, err := strconv.ParseInt(part, 10, 16)
		if err != nil {
			return developerVersionValue{}
		}
		numbers[index] = int16(parsed)
	}
	return developerVersionValue{numbers[0], numbers[1], numbers[2]}
}
