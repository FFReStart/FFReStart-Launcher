package patch

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
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const gameTestKeyID = "test-game-manifest"

func gameTestPrivateKey() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(255 - index)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func signedGameManifest(t *testing.T, version, path, artifactURL string, content []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(content)
	manifest := Manifest{Version: version, KeyID: gameTestKeyID, Files: []File{{Path: path, URL: artifactURL, Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:])}}}
	signed, err := manifest.SigningBytes()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Signature = base64.RawStdEncoding.EncodeToString(ed25519.Sign(gameTestPrivateKey(), signed))
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGameInstallKeepsPreviousAndRollsBackHealthFailure(t *testing.T) {
	artifacts := map[string][]byte{"/v1": []byte("known-good-game"), "/v2": []byte("unhealthy-game")}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = response.Write(artifacts[request.URL.Path])
	}))
	defer server.Close()
	root := t.TempDir()
	installer := Installer{Root: root, KeyID: gameTestKeyID, PublicKey: gameTestPrivateKey().Public().(ed25519.PublicKey)}
	if err := installer.Install(context.Background(), signedGameManifest(t, "v1.0.0", "bin/game", server.URL+"/v1", artifacts["/v1"])); err != nil {
		t.Fatal(err)
	}
	installer.HealthCheck = func(context.Context, string) error { return errors.New("game did not start") }
	begin := time.Now()
	if err := installer.Install(context.Background(), signedGameManifest(t, "v2.0.0", "bin/game", server.URL+"/v2", artifacts["/v2"])); err == nil {
		t.Fatal("unhealthy game version was accepted")
	}
	if elapsed := time.Since(begin); elapsed >= 30*time.Second {
		t.Fatalf("rollback took %s", elapsed)
	}
	current, err := installer.CurrentPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(current) != "1.0.0" {
		t.Fatalf("current version path = %q", current)
	}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		if _, err := os.Stat(filepath.Join(root, "versions", version, "bin", "game")); err != nil {
			t.Fatalf("version %s was not retained: %v", version, err)
		}
	}
}

func TestFailedGameUpdateLeavesCurrentVersion(t *testing.T) {
	content := []byte("known-good-game")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write(content) }))
	defer server.Close()
	installer := Installer{Root: t.TempDir(), KeyID: gameTestKeyID, PublicKey: gameTestPrivateKey().Public().(ed25519.PublicKey)}
	if err := installer.Install(context.Background(), signedGameManifest(t, "v1.0.0", "game", server.URL, content)); err != nil {
		t.Fatal(err)
	}
	manifest := signedGameManifest(t, "v2.0.0", "game", server.URL, []byte("different expected bytes"))
	if err := installer.Install(context.Background(), manifest); err == nil {
		t.Fatal("corrupted game update was accepted")
	}
	current, err := installer.CurrentPath()
	if err != nil || filepath.Base(current) != "1.0.0" {
		t.Fatalf("current path = %q, %v", current, err)
	}
}

func TestSignedGameManifestRejectsChangedFileMetadata(t *testing.T) {
	data := signedGameManifest(t, "v1.0.0", "game", "https://updates.example/game", []byte("game"))
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Files[0].Size++
	if err := VerifyManifest(manifest, gameTestKeyID, gameTestPrivateKey().Public().(ed25519.PublicKey)); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("error = %v, want signature rejection", err)
	}
}

func TestWindowsManifestPathsRejectCollisionsAndDevices(t *testing.T) {
	for _, files := range [][]File{
		{{Path: "Data/Game.dll", URL: "https://updates.example/a", Size: 1, SHA256: strings.Repeat("0", 64)}, {Path: "data/game.DLL", URL: "https://updates.example/b", Size: 1, SHA256: strings.Repeat("1", 64)}},
		{{Path: "bin/CON.txt", URL: "https://updates.example/a", Size: 1, SHA256: strings.Repeat("0", 64)}},
		{{Path: "aux/config", URL: "https://updates.example/a", Size: 1, SHA256: strings.Repeat("0", 64)}},
		{{Path: "COM1.dll", URL: "https://updates.example/a", Size: 1, SHA256: strings.Repeat("0", 64)}},
	} {
		manifest := Manifest{Version: "v1.0.0", KeyID: gameTestKeyID, Files: files, Signature: base64.RawStdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))}
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseManifestForTarget(data, "windows", false); !errors.Is(err, ErrInvalidManifest) {
			t.Fatalf("files %#v: error = %v, want rejection", files, err)
		}
	}
}

func TestReleaseGameManifestRequiresHTTPS(t *testing.T) {
	data := signedGameManifest(t, "v1.0.0", "game", "http://updates.example/game", []byte("game"))
	if _, err := ParseManifestForTarget(data, "linux", false); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("error = %v, want insecure URL rejection", err)
	}
	if _, err := ParseManifestForTarget(data, "linux", true); err != nil {
		t.Fatalf("local development HTTP rejected: %v", err)
	}
}

func TestReleaseInstallerRejectsHTTPManifestURL(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	installer := Installer{Root: t.TempDir(), Release: true}
	if err := installer.FetchAndInstall(context.Background(), server.Client(), server.URL); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("error = %v, want insecure manifest URL rejection", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("HTTP manifest service received %d requests", got)
	}
}
