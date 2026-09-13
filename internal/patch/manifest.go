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
	"path"
	"runtime"
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
	return ParseManifestForTarget(data, runtime.GOOS, true)
}

// ParseManifestForTarget validates paths for the target operating system.
// allowHTTP exists only for local development and tests.
func ParseManifestForTarget(data []byte, targetOS string, allowHTTP bool) (Manifest, error) {
	manifest, err := ParseUnsignedManifestForTarget(data, targetOS, allowHTTP)
	if err != nil {
		return Manifest{}, err
	}
	if signature, err := base64.RawStdEncoding.DecodeString(manifest.Signature); err != nil || len(signature) != ed25519.SignatureSize {
		return Manifest{}, ErrInvalidManifest
	}
	return manifest, nil
}

// ParseUnsignedManifestForTarget validates an unsigned signing input while
// preserving the file order that is covered by the signature.
func ParseUnsignedManifestForTarget(data []byte, targetOS string, allowHTTP bool) (Manifest, error) {
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
		clean := path.Clean(file.Path)
		parsedURL, err := url.ParseRequestURI(file.URL)
		comparisonPath := clean
		if targetOS == "windows" {
			comparisonPath = strings.ToLower(clean)
		}
		_, duplicate := seen[comparisonPath]
		unsafePath := strings.ContainsAny(file.Path, "\\:\x00")
		if err != nil || !allowedRemoteURL(parsedURL, allowHTTP) || unsafePath || clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || duplicate || (targetOS == "windows" && invalidWindowsPath(clean)) || file.Size <= 0 || file.Size > 256<<20 {
			return Manifest{}, ErrInvalidManifest
		}
		hash, err := hex.DecodeString(file.SHA256)
		if err != nil || len(hash) != sha256.Size {
			return Manifest{}, ErrInvalidManifest
		}
		seen[comparisonPath] = struct{}{}
	}
	return manifest, nil
}

func allowedRemoteURL(parsedURL *url.URL, allowHTTP bool) bool {
	return parsedURL.Host != "" && (parsedURL.Scheme == "https" || (allowHTTP && parsedURL.Scheme == "http"))
}

func invalidWindowsPath(filePath string) bool {
	for _, component := range strings.Split(filePath, "/") {
		if component == "" || strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
			return true
		}
		stem := strings.ToUpper(strings.TrimRight(strings.SplitN(component, ".", 2)[0], " ."))
		switch stem {
		case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9", "COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³":
			return true
		}
	}
	return false
}

// SigningBytes returns the exact canonical JSON covered by the signature.
func (m Manifest) SigningBytes() ([]byte, error) {
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
	signed, err := manifest.SigningBytes()
	if err != nil {
		return ErrInvalidManifest
	}
	signature, err := base64.RawStdEncoding.DecodeString(manifest.Signature)
	if err != nil || !ed25519.Verify(publicKey, signed, signature) {
		return ErrInvalidSignature
	}
	return nil
}
