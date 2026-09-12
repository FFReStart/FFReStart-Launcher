package updater

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	selfapply "github.com/creativeprojects/go-selfupdate/update"
	"golang.org/x/mod/semver"
)

// UpdatePublicKeyHex is compiled into the launcher. The matching private key is
// deliberately absent (RFC 8032 test-vector key for this disposable spike).
const UpdatePublicKeyHex = "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"

type Manifest struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
}

func (m Manifest) signedBytes() []byte {
	return []byte(m.Version + "\n" + m.URL + "\n" + stringsLower(m.SHA256) + "\n")
}
func stringsLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'F' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

func Sign(m *Manifest, private ed25519.PrivateKey) {
	m.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(private, m.signedBytes()))
}

func EmbeddedPublicKey() ed25519.PublicKey {
	b, err := hex.DecodeString(UpdatePublicKeyHex)
	if err != nil {
		panic(err)
	}
	return ed25519.PublicKey(b)
}

func FetchAndApply(ctx context.Context, manifestURL, currentVersion, targetPath string, public ed25519.PublicKey) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("manifest HTTP %d", resp.StatusCode)
	}
	var m Manifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return err
	}
	sig, err := base64.RawStdEncoding.DecodeString(m.Signature)
	if err != nil || !ed25519.Verify(public, m.signedBytes(), sig) {
		return errors.New("update manifest signature rejected")
	}
	if !semver.IsValid(m.Version) || !semver.IsValid(currentVersion) || semver.Compare(m.Version, currentVersion) <= 0 {
		return errors.New("update version is not newer")
	}

	req, _ = http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	binary, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(binary)
	if !stringsEqualFold(hex.EncodeToString(sum[:]), m.SHA256) {
		return errors.New("update binary SHA-256 rejected")
	}
	// The maintained go-selfupdate apply engine provides Windows rollback-safe
	// rename semantics. Passing the already verified bytes avoids a second fetch.
	return selfapply.Apply(bytes.NewReader(binary), selfapply.Options{TargetPath: targetPath})
}

func stringsEqualFold(a, b string) bool { return stringsLower(a) == stringsLower(b) }
