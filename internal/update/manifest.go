package update

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"golang.org/x/mod/semver"
)

var (
	ErrInvalidManifest  = errors.New("invalid update manifest")
	ErrInvalidSignature = errors.New("update manifest signature rejected")
	ErrNotNewer         = errors.New("update version is not newer")
	ErrTestReleaseKey   = errors.New("release build refuses a known test update key")
	ErrInvalidSize      = errors.New("update size is invalid")
)

// knownTestPublicKeys contains public material only. Keep every key used by
// tests and spikes here so renaming a key ID cannot make it release-trusted.
var knownTestPublicKeys = [...][ed25519.PublicKeySize]byte{
	// Public half of the deterministic key used by release-validation tests.
	{0x79, 0xb5, 0x56, 0x2e, 0x8f, 0xe6, 0x54, 0xf9, 0x40, 0x78, 0xb1, 0x12, 0xe8, 0xa9, 0x8b, 0xa7, 0x90, 0x1f, 0x85, 0x3a, 0xe6, 0x95, 0xbe, 0xd7, 0xe0, 0xe3, 0x91, 0x0b, 0xad, 0x04, 0x96, 0x64},
	// RFC 8032 test vector 1, used by origin/wails-spike.
	{0xd7, 0x5a, 0x98, 0x01, 0x82, 0xb1, 0x0a, 0xb7, 0xd5, 0x4b, 0xfe, 0xd3, 0xc9, 0x64, 0x07, 0x3a, 0x0e, 0xe1, 0x72, 0xf3, 0xda, 0xa6, 0x23, 0x25, 0xaf, 0x02, 0x1a, 0x68, 0xf7, 0x07, 0x51, 0x1a},
	// Public half of the deterministic game-manifest test key.
	{0xba, 0xfc, 0x71, 0xbe, 0xad, 0x3a, 0xc5, 0xe4, 0xb6, 0x3e, 0x9c, 0x82, 0x16, 0xee, 0x71, 0xa3, 0x4a, 0xae, 0xc6, 0x57, 0x22, 0xee, 0xdb, 0xca, 0x72, 0x8b, 0x4e, 0x9b, 0x3c, 0xcc, 0xe3, 0x96},
}

type Manifest struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

func ParseManifest(data []byte) (Manifest, error) {
	return ParseManifestWithPolicy(data, true)
}

// ParseManifestWithPolicy parses a launcher manifest. allowHTTP exists only
// for local development and tests; release callers must pass false.
func ParseManifestWithPolicy(data []byte, allowHTTP bool) (Manifest, error) {
	manifest, err := ParseUnsignedManifestWithPolicy(data, allowHTTP)
	if err != nil {
		return Manifest{}, err
	}
	if signature, err := base64.RawStdEncoding.DecodeString(manifest.Signature); err != nil || len(signature) != ed25519.SignatureSize {
		return Manifest{}, ErrInvalidManifest
	}
	return manifest, nil
}

// ParseUnsignedManifestWithPolicy validates an unsigned signing input. The
// signature field may be absent, but all signed fields remain mandatory.
func ParseUnsignedManifestWithPolicy(data []byte, allowHTTP bool) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Manifest{}, err
	}
	parsedURL, err := url.ParseRequestURI(manifest.URL)
	if err != nil || !allowedRemoteURL(parsedURL, allowHTTP) || !semver.IsValid(manifest.Version) || len(manifest.KeyID) == 0 || len(manifest.KeyID) > 128 {
		return Manifest{}, ErrInvalidManifest
	}
	if manifest.Size <= 0 || manifest.Size > maxArtifactSize {
		return Manifest{}, ErrInvalidSize
	}
	hash, err := hex.DecodeString(manifest.SHA256)
	if err != nil || len(hash) != 32 {
		return Manifest{}, ErrInvalidManifest
	}
	return manifest, nil
}

func allowedRemoteURL(parsedURL *url.URL, allowHTTP bool) bool {
	return parsedURL.Host != "" && (parsedURL.Scheme == "https" || (allowHTTP && parsedURL.Scheme == "http"))
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrInvalidManifest
	}
	return nil
}

// SigningBytes returns the exact bytes covered by the Ed25519 signature.
func (m Manifest) SigningBytes() []byte {
	return []byte(m.Version + "\n" + m.URL + "\n" + fmt.Sprintf("%d", m.Size) + "\n" + strings.ToLower(m.SHA256) + "\n" + m.KeyID + "\n")
}

func VerifyManifest(manifest Manifest, currentVersion, expectedKeyID string, publicKey ed25519.PublicKey) error {
	if err := VerifySignature(manifest, expectedKeyID, publicKey); err != nil {
		return err
	}
	if !semver.IsValid(currentVersion) || semver.Compare(manifest.Version, currentVersion) <= 0 {
		return ErrNotNewer
	}
	return nil
}

// VerifySignature verifies authenticity without applying update-version policy.
func VerifySignature(manifest Manifest, expectedKeyID string, publicKey ed25519.PublicKey) error {
	if manifest.KeyID != expectedKeyID || len(publicKey) != ed25519.PublicKeySize {
		return ErrInvalidSignature
	}
	signature, err := base64.RawStdEncoding.DecodeString(manifest.Signature)
	if err != nil || !ed25519.Verify(publicKey, manifest.SigningBytes(), signature) {
		return ErrInvalidSignature
	}
	return nil
}

func ReleasePublicKey(keyID, encoded string, release bool) (ed25519.PublicKey, error) {
	if strings.TrimSpace(encoded) == "" {
		if release {
			return nil, errors.New("release build requires an injected update public key")
		}
		return nil, nil
	}
	key, err := hex.DecodeString(encoded)
	if err != nil || len(key) != ed25519.PublicKeySize || keyID == "" {
		return nil, errors.New("invalid injected update public key")
	}
	if release {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(keyID)), "test") || isKnownTestPublicKey(key) {
			return nil, ErrTestReleaseKey
		}
	}
	return ed25519.PublicKey(key), nil
}

func isKnownTestPublicKey(key []byte) bool {
	for _, denied := range knownTestPublicKeys {
		if bytes.Equal(key, denied[:]) {
			return true
		}
	}
	return false
}
