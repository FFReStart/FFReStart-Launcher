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
)

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
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(keyID)), "test") && release {
		return nil, errors.New("release build refuses a test update key")
	}
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
	return ed25519.PublicKey(key), nil
}
