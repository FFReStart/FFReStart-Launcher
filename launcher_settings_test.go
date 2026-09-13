package main

import (
	"math"
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
