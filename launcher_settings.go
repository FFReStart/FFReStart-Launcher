package main

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FFReStart/FFReStart-Launcher/internal/update"
)

const defaultMusicVolume = 0.35

type LauncherSettings struct {
	Version          int     `json:"version"`
	SetupComplete    bool    `json:"setupComplete"`
	InstallDirectory string  `json:"installDirectory"`
	MusicVolume      float64 `json:"musicVolume"`
	MusicMuted       bool    `json:"musicMuted"`
}

type settingsStore struct {
	mu   sync.Mutex
	path string
}

func newSettingsStore() (*settingsStore, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return &settingsStore{path: filepath.Join(root, "FFReStart", "launcher", "settings.json")}, nil
}

func defaultSettings(gameRoot string) LauncherSettings {
	return LauncherSettings{Version: 1, InstallDirectory: gameRoot, MusicVolume: defaultMusicVolume}
}

func (s *settingsStore) Load(gameRoot string) LauncherSettings {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings := defaultSettings(gameRoot)
	data, err := os.ReadFile(s.path) // #nosec G304 -- fixed per-user launcher path.
	if err != nil || json.Unmarshal(data, &settings) != nil || settings.Version != 1 {
		return defaultSettings(gameRoot)
	}
	if strings.TrimSpace(settings.InstallDirectory) == "" || !filepath.IsAbs(settings.InstallDirectory) {
		settings.InstallDirectory = gameRoot
	} else {
		settings.InstallDirectory = filepath.Clean(settings.InstallDirectory)
	}
	settings.MusicVolume = normaliseVolume(settings.MusicVolume)
	return settings
}

func (s *settingsStore) Save(settings LauncherSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings.Version = 1
	settings.MusicVolume = normaliseVolume(settings.MusicVolume)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return update.AtomicWriteFile(s.path, data, 0o600)
}

func normaliseVolume(volume float64) float64 {
	if math.IsNaN(volume) || math.IsInf(volume, 0) {
		return defaultMusicVolume
	}
	return math.Max(0, math.Min(1, volume))
}

func validateInstallRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("an install folder is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", err
	}
	probe, err := os.CreateTemp(abs, ".ffrestart-write-*")
	if err != nil {
		return "", errors.New("the selected install folder is not writable")
	}
	name := probe.Name()
	closeErr := probe.Close()
	removeErr := os.Remove(name)
	if closeErr != nil || removeErr != nil {
		return "", errors.Join(closeErr, removeErr)
	}
	return filepath.Clean(abs), nil
}
