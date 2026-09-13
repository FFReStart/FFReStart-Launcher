package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
)

const (
	legacyBuildDirectory = "FFReStart-Dev-Build"
	legacyWindowsGame    = "FFReStart-Dev-Build.exe"
	legacyVersionFile    = "Version.txt"
)

type gameInstallation struct{ installer *patch.Installer }

func (g *gameInstallation) CurrentPath() (string, error) {
	if g == nil || g.installer == nil {
		return "", errors.New("game installation is unavailable")
	}
	if current, err := g.installer.CurrentPath(); err == nil && current != "" {
		return current, nil
	}
	root := g.installer.Root
	if usableExecutable(legacyExecutablePath(root)) {
		return filepath.Join(root, legacyBuildDirectory), nil
	}
	backup := filepath.Join(root, legacyBuildDirectory+".previous")
	if usableExecutable(filepath.Join(backup, legacyExecutableName())) {
		return backup, nil
	}
	return "", errors.New("game is not installed")
}

func legacyExecutableName() string {
	if legacy := os.Getenv("FFRESTART_LEGACY_GAME_EXECUTABLE"); strings.TrimSpace(legacy) != "" {
		return filepath.Base(strings.TrimSpace(legacy))
	}
	return legacyWindowsGame
}

func legacyExecutablePath(root string) string {
	return filepath.Join(root, legacyBuildDirectory, legacyExecutableName())
}

func legacyVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, legacyVersionFile)) // #nosec G304 -- root is the user-selected install directory.
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func nearestExistingDirectory(path string) string {
	path = filepath.Clean(path)
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}
