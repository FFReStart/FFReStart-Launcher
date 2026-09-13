package update

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"

	selfapply "github.com/creativeprojects/go-selfupdate/update"
)

type HTTPChecker struct {
	client         *http.Client
	manifestURL    string
	currentVersion string
	keyID          string
	publicKey      ed25519.PublicKey
	manifestLimit  int64
}

func NewHTTPChecker(manifestURL, currentVersion, keyID string, publicKey ed25519.PublicKey, manifestLimit int64) *HTTPChecker {
	return &HTTPChecker{client: http.DefaultClient, manifestURL: manifestURL, currentVersion: currentVersion, keyID: keyID, publicKey: publicKey, manifestLimit: manifestLimit}
}

// SetKeyID makes the allowed release key explicit. It is primarily useful for
// tests and future key rotation; a manifest cannot select an arbitrary key.
func (c *HTTPChecker) SetKeyID(keyID string) { c.keyID = keyID }

func (c *HTTPChecker) Check(ctx context.Context) error {
	_, err := c.fetchManifest(ctx)
	return err
}

func (c *HTTPChecker) FetchAndApply(ctx context.Context, targetPath string) error {
	manifest, err := c.fetchManifest(ctx)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifest.URL, nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("update download HTTP %d", response.StatusCode)
	}
	binary, err := io.ReadAll(io.LimitReader(response.Body, (256<<20)+1))
	if err != nil {
		return err
	}
	if len(binary) > 256<<20 {
		return errors.New("update binary exceeds size limit")
	}
	sum := sha256.Sum256(binary)
	if !stringsEqualFold(hex.EncodeToString(sum[:]), manifest.SHA256) {
		return errors.New("update binary SHA-256 rejected")
	}
	return selfapply.Apply(bytes.NewReader(binary), selfapply.Options{TargetPath: targetPath})
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

func stringsEqualFold(left, right string) bool {
	return len(left) == len(right) && bytes.EqualFold([]byte(left), []byte(right))
}
