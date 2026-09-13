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
)

// knownTestPublicKeys contains public material only. Keep every key used by
// tests and spikes here so renaming a key ID cannot make it release-trusted.
var knownTestPublicKeys = [...][ed25519.PublicKeySize]byte{
	// Public half of the deterministic key used by release-validation tests.
	{0x79, 0xb5, 0x56, 0x2e, 0x8f, 0xe6, 0x54, 0xf9, 0x40, 0x78, 0xb1, 0x12, 0xe8, 0xa9, 0x8b, 0xa7, 0x90, 0x1f, 0x85, 0x3a, 0xe6, 0x95, 0xbe, 0xd7, 0xe0, 0xe3, 0x91, 0x0b, 0xad, 0x04, 0x96, 0x64},
	// RFC 8032 test vector 1, used by origin/wails-spike.
	{0xd7, 0x5a, 0x98, 0x01, 0x82, 0xb1, 0x0a, 0xb7, 0xd5, 0x4b, 0xfe, 0xd3, 0xc9, 0x64, 0x07, 0x3a, 0x0e, 0xe1, 0x72, 0xf3, 0xda, 0xa6, 0x23, 0x25, 0xaf, 0x02, 0x1a, 0x68, 0xf7, 0x07, 0x51, 0x1a},
}

type Manifest struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

func ParseManifest(data []byte) (Manifest, error) {
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
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" || !semver.IsValid(manifest.Version) || len(manifest.KeyID) == 0 || len(manifest.KeyID) > 128 {
		return Manifest{}, ErrInvalidManifest
	}
	hash, err := hex.DecodeString(manifest.SHA256)
	if err != nil || len(hash) != 32 {
		return Manifest{}, ErrInvalidManifest
	}
	if _, err := base64.RawStdEncoding.DecodeString(manifest.Signature); err != nil {
		return Manifest{}, ErrInvalidManifest
	}
	return manifest, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrInvalidManifest
	}
	return nil
}

func (m Manifest) signedBytes() []byte {
	return []byte(m.Version + "\n" + m.URL + "\n" + strings.ToLower(m.SHA256) + "\n" + m.KeyID + "\n")
}

func VerifyManifest(manifest Manifest, currentVersion, expectedKeyID string, publicKey ed25519.PublicKey) error {
	if manifest.KeyID != expectedKeyID || len(publicKey) != ed25519.PublicKeySize {
		return ErrInvalidSignature
	}
	signature, err := base64.RawStdEncoding.DecodeString(manifest.Signature)
	if err != nil || !ed25519.Verify(publicKey, manifest.signedBytes(), signature) {
		return ErrInvalidSignature
	}
	if !semver.IsValid(currentVersion) || semver.Compare(manifest.Version, currentVersion) <= 0 {
		return ErrNotNewer
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
