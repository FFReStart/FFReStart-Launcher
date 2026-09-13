package patch

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

var (
	ErrInvalidManifest  = errors.New("invalid game manifest")
	ErrInvalidSignature = errors.New("game manifest signature rejected")
	ErrNotNewer         = errors.New("game version is not newer")
)

type File struct {
	Path   string `json:"path"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Version   string `json:"version"`
	KeyID     string `json:"key_id"`
	Files     []File `json:"files"`
	Signature string `json:"signature"`
}

func ParseManifest(data []byte) (Manifest, error) {
	if len(data) == 0 || len(data) > 1<<20 {
		return Manifest{}, ErrInvalidManifest
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Manifest{}, ErrInvalidManifest
	}
	if !semver.IsValid(manifest.Version) || manifest.KeyID == "" || len(manifest.KeyID) > 128 || len(manifest.Files) == 0 || len(manifest.Files) > 10000 {
		return Manifest{}, ErrInvalidManifest
	}
	seen := make(map[string]struct{}, len(manifest.Files))
	for _, file := range manifest.Files {
		clean := filepath.Clean(filepath.FromSlash(file.Path))
		parsedURL, err := url.ParseRequestURI(file.URL)
		_, duplicate := seen[clean]
		unsafePath := strings.ContainsAny(file.Path, "\\:\x00")
		if err != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" || unsafePath || clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || duplicate || file.Size <= 0 || file.Size > 256<<20 {
			return Manifest{}, ErrInvalidManifest
		}
		hash, err := hex.DecodeString(file.SHA256)
		if err != nil || len(hash) != sha256.Size {
			return Manifest{}, ErrInvalidManifest
		}
		seen[clean] = struct{}{}
	}
	if signature, err := base64.RawStdEncoding.DecodeString(manifest.Signature); err != nil || len(signature) != ed25519.SignatureSize {
		return Manifest{}, ErrInvalidManifest
	}
	return manifest, nil
}

func (m Manifest) signedBytes() ([]byte, error) {
	unsigned := struct {
		Version string `json:"version"`
		KeyID   string `json:"key_id"`
		Files   []File `json:"files"`
	}{m.Version, m.KeyID, m.Files}
	return json.Marshal(unsigned)
}

func VerifyManifest(manifest Manifest, expectedKeyID string, publicKey ed25519.PublicKey) error {
	if manifest.KeyID != expectedKeyID || len(publicKey) != ed25519.PublicKeySize {
		return ErrInvalidSignature
	}
	signed, err := manifest.signedBytes()
	if err != nil {
		return ErrInvalidManifest
	}
	signature, err := base64.RawStdEncoding.DecodeString(manifest.Signature)
	if err != nil || !ed25519.Verify(publicKey, signed, signature) {
		return ErrInvalidSignature
	}
	return nil
}
