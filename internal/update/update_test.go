package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const testOnlyKeyID = "test-only-rfc8032"

func testOnlyPrivateKey() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func signTestOnly(manifest *Manifest) {
	manifest.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(testOnlyPrivateKey(), manifest.signedBytes()))
}

func updateServer(t *testing.T, binary []byte, mutate func(*Manifest)) (*httptest.Server, string) {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/launcher" {
			_, _ = response.Write(binary)
			return
		}
		sum := sha256.Sum256(binary)
		manifest := Manifest{Version: "v2.0.0", URL: server.URL + "/launcher", SHA256: hex.EncodeToString(sum[:]), KeyID: testOnlyKeyID}
		signTestOnly(&manifest)
		if mutate != nil {
			mutate(&manifest)
		}
		_ = json.NewEncoder(response).Encode(manifest)
	}))
	return server, server.URL + "/manifest.json"
}

func TestSignedUpdateDownloadsHashesAndApplies(t *testing.T) {
	binary := []byte("verified replacement")
	server, manifestURL := updateServer(t, binary, nil)
	defer server.Close()
	target := filepath.Join(t.TempDir(), "launcher")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	checker := NewHTTPChecker(manifestURL, "v1.0.0", testOnlyKeyID, testOnlyPrivateKey().Public().(ed25519.PublicKey), 1<<20)
	if err := checker.FetchAndApply(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target) // #nosec G304 -- target is inside t.TempDir.
	if err != nil || string(got) != string(binary) {
		t.Fatalf("replacement = %q, %v", got, err)
	}
}

func TestTamperedManifestIsRejected(t *testing.T) {
	server, manifestURL := updateServer(t, []byte("good"), func(manifest *Manifest) { manifest.URL += "?tampered=true" })
	defer server.Close()
	checker := NewHTTPChecker(manifestURL, "v1.0.0", testOnlyKeyID, testOnlyPrivateKey().Public().(ed25519.PublicKey), 1<<20)
	if err := checker.Check(context.Background()); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("error = %v, want signature rejection", err)
	}
}

func TestTamperedBinaryIsRejected(t *testing.T) {
	server, manifestURL := updateServer(t, []byte("actual"), func(manifest *Manifest) {
		sum := sha256.Sum256([]byte("expected"))
		manifest.SHA256 = hex.EncodeToString(sum[:])
		signTestOnly(manifest)
	})
	defer server.Close()
	target := filepath.Join(t.TempDir(), "launcher")
	_ = os.WriteFile(target, []byte("old"), 0o600)
	checker := NewHTTPChecker(manifestURL, "v1.0.0", testOnlyKeyID, testOnlyPrivateKey().Public().(ed25519.PublicKey), 1<<20)
	if err := checker.FetchAndApply(context.Background(), target); err == nil {
		t.Fatal("tampered binary was accepted")
	}
}

func TestEqualAndDowngradeVersionsAreRejected(t *testing.T) {
	for _, version := range []string{"v2.0.0", "v3.0.0"} {
		server, manifestURL := updateServer(t, []byte("old"), nil)
		checker := NewHTTPChecker(manifestURL, version, testOnlyKeyID, testOnlyPrivateKey().Public().(ed25519.PublicKey), 1<<20)
		if err := checker.Check(context.Background()); !errors.Is(err, ErrNotNewer) {
			t.Errorf("current %s: error = %v", version, err)
		}
		server.Close()
	}
}

func TestReleaseBuildRequiresProductionKey(t *testing.T) {
	if _, err := ReleasePublicKey(testOnlyKeyID, hex.EncodeToString(testOnlyPrivateKey().Public().(ed25519.PublicKey)), true); err == nil {
		t.Fatal("release accepted a test-only key")
	}
	if _, err := ReleasePublicKey("release-2026-01", "", true); err == nil {
		t.Fatal("release accepted a missing key")
	}
}
