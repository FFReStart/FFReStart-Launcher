package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestLauncherSettingsPersistInstallAndMusic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store := &settingsStore{path: path}
	want := LauncherSettings{Version: 1, SetupComplete: true, InstallDirectory: filepath.Join(t.TempDir(), "game"), MusicVolume: 0.62, MusicMuted: true}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got := store.Load("unused")
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestLauncherSettingsMigratesObsoleteGameSubfolder(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "settings.json")
	store := &settingsStore{path: path}
	data, err := json.Marshal(LauncherSettings{Version: 1, SetupComplete: true, InstallDirectory: filepath.Join(root, "game"), MusicVolume: defaultMusicVolume})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := store.Load(root).InstallDirectory; got != root {
		t.Fatalf("InstallDirectory = %q, want legacy root %q", got, root)
	}
}

func TestNormaliseMusicVolume(t *testing.T) {
	if got := normaliseVolume(math.NaN()); got != defaultMusicVolume {
		t.Fatalf("NaN = %v", got)
	}
	if got := normaliseVolume(2); got != 1 {
		t.Fatalf("high volume = %v", got)
	}
	if got := normaliseVolume(-1); got != 0 {
		t.Fatalf("low volume = %v", got)
	}
}
