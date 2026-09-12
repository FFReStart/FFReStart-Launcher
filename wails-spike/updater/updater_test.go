package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testPrivateKey(t *testing.T) ed25519.PrivateKey {
	seed, err := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	if err != nil {
		t.Fatal(err)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func updateServer(t *testing.T, binary []byte, mutateManifest func(*Manifest)) (*httptest.Server, string) {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/launcher.exe" {
			_, _ = w.Write(binary)
			return
		}
		sum := sha256.Sum256(binary)
		m := Manifest{Version: "v2.0.0", URL: server.URL + "/launcher.exe", SHA256: hex.EncodeToString(sum[:])}
		Sign(&m, testPrivateKey(t))
		if mutateManifest != nil {
			mutateManifest(&m)
		}
		json.NewEncoder(w).Encode(m)
	}))
	return server, server.URL + "/manifest.json"
}

func TestSignedUpdateDownloadsHashesAndReplaces(t *testing.T) {
	server, url := updateServer(t, []byte("new verified launcher bytes"), nil)
	defer server.Close()
	target := filepath.Join(t.TempDir(), "launcher.exe")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := FetchAndApply(context.Background(), url, "v1.0.0", target, EmbeddedPublicKey()); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "new verified launcher bytes" {
		t.Fatalf("replacement failed: %q", got)
	}
}

func TestTamperedManifestRejected(t *testing.T) {
	server, url := updateServer(t, []byte("good"), func(m *Manifest) { m.URL += "?tampered=1" })
	defer server.Close()
	target := filepath.Join(t.TempDir(), "launcher.exe")
	_ = os.WriteFile(target, []byte("old"), 0o755)
	if err := FetchAndApply(context.Background(), url, "v1.0.0", target, EmbeddedPublicKey()); err == nil {
		t.Fatal("tampered manifest accepted")
	}
}

func TestTamperedBinaryRejected(t *testing.T) {
	good := []byte("good")
	server, url := updateServer(t, good, func(m *Manifest) {
		sum := sha256.Sum256([]byte("different"))
		m.SHA256 = hex.EncodeToString(sum[:])
		Sign(m, testPrivateKey(t))
	})
	defer server.Close()
	target := filepath.Join(t.TempDir(), "launcher.exe")
	_ = os.WriteFile(target, []byte("old"), 0o755)
	if err := FetchAndApply(context.Background(), url, "v1.0.0", target, EmbeddedPublicKey()); err == nil {
		t.Fatal("tampered binary accepted")
	}
}

func TestDowngradeRejected(t *testing.T) {
	server, url := updateServer(t, []byte("old release"), func(m *Manifest) { m.Version = "v1.9.0"; Sign(m, testPrivateKey(t)) })
	defer server.Close()
	target := filepath.Join(t.TempDir(), "launcher.exe")
	_ = os.WriteFile(target, []byte("current"), 0o755)
	if err := FetchAndApply(context.Background(), url, "v2.0.0", target, EmbeddedPublicKey()); err == nil {
		t.Fatal("downgrade accepted")
	}
}
