package update

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	selfapply "github.com/creativeprojects/go-selfupdate/update"
)

type HTTPChecker struct {
	client         *http.Client
	manifestURL    string
	currentVersion string
	keyID          string
	publicKey      ed25519.PublicKey
	manifestLimit  int64
	healthCheck    func(context.Context, string) error
}

func NewHTTPChecker(manifestURL, currentVersion, keyID string, publicKey ed25519.PublicKey, manifestLimit int64) *HTTPChecker {
	return &HTTPChecker{client: http.DefaultClient, manifestURL: manifestURL, currentVersion: currentVersion, keyID: keyID, publicKey: publicKey, manifestLimit: manifestLimit}
}

// SetKeyID makes the allowed release key explicit. It is primarily useful for
// tests and future key rotation; a manifest cannot select an arbitrary key.
func (c *HTTPChecker) SetKeyID(keyID string) { c.keyID = keyID }

func (c *HTTPChecker) SetClient(client *http.Client) { c.client = client }

// SetHealthCheck configures a bounded check after launcher replacement. A
// failure restores the retained previous launcher before returning.
func (c *HTTPChecker) SetHealthCheck(check func(context.Context, string) error) {
	c.healthCheck = check
}

func (c *HTTPChecker) Check(ctx context.Context) error {
	_, err := c.fetchManifest(ctx)
	return err
}

func (c *HTTPChecker) FetchAndApply(ctx context.Context, targetPath string) error {
	manifest, err := c.fetchManifest(ctx)
	if err != nil {
		return err
	}
	downloadPath := targetPath + ".download"
	if err := (Downloader{Client: c.client}).Download(ctx, manifest.URL, downloadPath, manifest.Size, manifest.SHA256); err != nil {
		return err
	}
	defer func() { _ = os.Remove(downloadPath) }()
	previousPath := targetPath + ".previous"
	if err := copyFile(targetPath, previousPath); err != nil {
		return fmt.Errorf("retain previous launcher: %w", err)
	}
	if err := applyFile(downloadPath, targetPath); err != nil {
		return errors.Join(err, restoreLauncher(previousPath, targetPath))
	}
	if c.healthCheck != nil {
		healthCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = c.healthCheck(healthCtx, targetPath)
		cancel()
		if err != nil {
			return errors.Join(fmt.Errorf("launcher health check: %w", err), restoreLauncher(previousPath, targetPath))
		}
	}
	return nil
}

func applyFile(source, target string) error {
	file, err := os.Open(source) // #nosec G304 -- paths are application-controlled.
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return selfapply.Apply(file, selfapply.Options{TargetPath: target})
}

func restoreLauncher(previous, target string) error {
	if err := applyFile(previous, target); err != nil {
		return fmt.Errorf("restore previous launcher: %w", err)
	}
	return nil
}

func copyFile(source, destination string) error {
	in, err := os.Open(source) // #nosec G304 -- paths are application-controlled.
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	temporary := destination + ".tmp"
	out, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm()) // #nosec G304 -- paths are application-controlled.
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return atomicReplace(temporary, destination)
}

func (c *HTTPChecker) fetchManifest(ctx context.Context) (Manifest, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.manifestURL, nil)
	if err != nil {
		return Manifest{}, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return Manifest{}, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("manifest HTTP %d", response.StatusCode)
	}
	limit := c.manifestLimit
	if limit <= 0 {
		limit = 1 << 20
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return Manifest{}, err
	}
	if int64(len(data)) > limit {
		return Manifest{}, errors.New("update manifest exceeds size limit")
	}
	manifest, err := ParseManifest(data)
	if err != nil {
		return Manifest{}, err
	}
	if err := VerifyManifest(manifest, c.currentVersion, c.keyID, c.publicKey); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
