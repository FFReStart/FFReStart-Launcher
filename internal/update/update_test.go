package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		manifest := Manifest{Version: "v2.0.0", URL: server.URL + "/launcher", Size: int64(len(binary)), SHA256: hex.EncodeToString(sum[:]), KeyID: testOnlyKeyID}
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

func TestLauncherHealthFailureRestoresPreviousVersion(t *testing.T) {
	server, manifestURL := updateServer(t, []byte("unhealthy replacement"), nil)
	defer server.Close()
	target := filepath.Join(t.TempDir(), "launcher")
	if err := os.WriteFile(target, []byte("known good"), 0o600); err != nil {
		t.Fatal(err)
	}
	checker := NewHTTPChecker(manifestURL, "v1.0.0", testOnlyKeyID, testOnlyPrivateKey().Public().(ed25519.PublicKey), 1<<20)
	checker.SetHealthCheck(func(context.Context, string) error { return errors.New("startup failed") })
	begin := time.Now()
	if err := checker.FetchAndApply(context.Background(), target); err == nil {
		t.Fatal("unhealthy launcher was accepted")
	}
	if elapsed := time.Since(begin); elapsed >= 30*time.Second {
		t.Fatalf("rollback took %s", elapsed)
	}
	got, err := os.ReadFile(target) // #nosec G304 -- target is inside t.TempDir.
	if err != nil || string(got) != "known good" {
		t.Fatalf("restored launcher = %q, %v", got, err)
	}
	previous, err := os.ReadFile(target + ".previous") // #nosec G304 -- target is inside t.TempDir.
	if err != nil || string(previous) != "known good" {
		t.Fatalf("retained launcher = %q, %v", previous, err)
	}
}

func TestOneThousandManifestMutationsAreRejected(t *testing.T) {
	sum := sha256.Sum256([]byte("launcher"))
	original := Manifest{Version: "v2.0.0", URL: "https://updates.example/launcher", Size: 8, SHA256: hex.EncodeToString(sum[:]), KeyID: testOnlyKeyID}
	signTestOnly(&original)
	for index := range 1000 {
		mutated := original
		switch index % 6 {
		case 0:
			mutated.Signature = "A" + mutated.Signature[1:]
		case 1:
			mutated.Version = fmt.Sprintf("v2.0.%d", index+1)
		case 2:
			mutated.Size++
		case 3:
			mutated.SHA256 = strings.Repeat("0", 64)
		case 4:
			mutated.URL += fmt.Sprintf("?mutation=%d", index)
		case 5:
			mutated.KeyID = "release-other"
		}
		data, err := json.Marshal(mutated)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := ParseManifest(data)
		if err == nil {
			err = VerifyManifest(parsed, "v1.0.0", testOnlyKeyID, testOnlyPrivateKey().Public().(ed25519.PublicKey))
		}
		if err == nil {
			t.Fatalf("mutation %d was accepted", index)
		}
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

func TestReleaseBuildRejectsKnownTestKeyUnderProductionID(t *testing.T) {
	publicKey := testOnlyPrivateKey().Public().(ed25519.PublicKey)
	if _, err := ReleasePublicKey("release-2026-01", hex.EncodeToString(publicKey), true); !errors.Is(err, ErrTestReleaseKey) {
		t.Fatalf("error = %v, want known test key rejection", err)
	}
}

func TestReleaseBuildRejectsSpikeRFC8032KeyUnderProductionID(t *testing.T) {
	const spikePublicKey = "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a"
	if _, err := ReleasePublicKey("release-2026-01", spikePublicKey, true); !errors.Is(err, ErrTestReleaseKey) {
		t.Fatalf("error = %v, want spike test key rejection", err)
	}
}
