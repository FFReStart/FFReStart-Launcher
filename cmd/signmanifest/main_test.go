package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func signerTestPrivateKey() ed25519.PrivateKey {
	seed := make([]byte, ed25519.SeedSize)
	for index := range seed {
		seed[index] = byte(index + 1)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func TestSignAndVerifyLauncherManifestWithTestKey(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "unsigned.json")
	output := filepath.Join(directory, "signed.json")
	if err := os.WriteFile(input, []byte(`{"version":"v1.2.3","url":"http://localhost/launcher","size":12,"sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","key_id":"test-only"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	privateHex := hex.EncodeToString(signerTestPrivateKey())
	publicHex := hex.EncodeToString(signerTestPrivateKey().Public().(ed25519.PublicKey))
	getenv := func(name string) string {
		if name == "PRIVATE" {
			return privateHex
		}
		return publicHex
	}
	if err := run([]string{"sign", "-type", "launcher", "-manifest", input, "-output", output, "-key-env", "PRIVATE", "-development"}, getenv); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"verify", "-type", "launcher", "-manifest", output, "-key-env", "PUBLIC", "-development"}, getenv); err != nil {
		t.Fatal(err)
	}
}

func TestProductionSigningRefusesKnownTestKey(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "unsigned.json")
	output := filepath.Join(directory, "signed.json")
	if err := os.WriteFile(input, []byte(`{"version":"v1.2.3","url":"https://updates.example/launcher","size":12,"sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","key_id":"release-2026"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	secret := hex.EncodeToString(signerTestPrivateKey())
	err := run([]string{"sign", "-type", "launcher", "-manifest", input, "-output", output, "-key-env", "PRIVATE"}, func(string) string { return secret })
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("test key refusal did not safely fail: %v", err)
	}
}

func TestSignAndVerifyGameManifestFromKeyFiles(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "game-unsigned.json")
	output := filepath.Join(directory, "game-signed.json")
	privatePath := filepath.Join(directory, "private.key")
	publicPath := filepath.Join(directory, "public.key")
	manifest := `{"version":"v1.2.3","key_id":"test-game","files":[{"path":"bin/game.exe","url":"http://localhost/game.exe","size":4,"sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}]}`
	for path, data := range map[string]string{
		input:       manifest,
		privatePath: hex.EncodeToString(signerTestPrivateKey()),
		publicPath:  hex.EncodeToString(signerTestPrivateKey().Public().(ed25519.PublicKey)),
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := run([]string{"sign", "-type", "game", "-target-os", "windows", "-manifest", input, "-output", output, "-key-file", privatePath, "-development"}, os.Getenv); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"verify", "-type", "game", "-target-os", "windows", "-manifest", output, "-key-file", publicPath, "-development"}, os.Getenv); err != nil {
		t.Fatal(err)
	}
}

func TestAbsentEnvironmentKeyFailsClearly(t *testing.T) {
	err := run([]string{"verify", "-type", "launcher", "-manifest", "unused", "-key-env", "MISSING"}, func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "absent") {
		t.Fatalf("error = %v", err)
	}
}
