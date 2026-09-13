package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
)

func TestLegacyGameInstallationAndVersion(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	build := filepath.Join(root, legacyBuildDirectory)
	if err := os.MkdirAll(build, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, legacyWindowsGame), []byte("game"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, legacyVersionFile), []byte("1.2.3\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	installation := &gameInstallation{installer: &patch.Installer{Root: root}}
	got, err := installation.CurrentPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != build {
		t.Fatalf("CurrentPath() = %q, want %q", got, build)
	}
	if got := legacyVersion(root); got != "1.2.3" {
		t.Fatalf("legacyVersion() = %q", got)
	}
}

func TestLegacyGameStatusAndOfflineLaunch(t *testing.T) {
	root := t.TempDir()
	build := filepath.Join(root, legacyBuildDirectory)
	if err := os.MkdirAll(build, 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(build, legacyWindowsGame)
	if err := os.WriteFile(executable, []byte("game"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, legacyVersionFile), []byte("0.0.21"), 0o600); err != nil {
		t.Fatal(err)
	}
	var startedPath string
	var startedArguments []string
	service := launch.NewService("", launch.FuncStarter(func(_ context.Context, path string, arguments ...string) error {
		startedPath, startedArguments = path, arguments
		return nil
	}), nil)
	installer := &patch.Installer{Root: root}
	service.SetInstalledGame(&gameInstallation{installer: installer}, legacyWindowsGame)
	app := NewApp(service, nil)
	app.ConfigureInstaller(installer, "", nil)
	app.ConfigureExperience(nil, LauncherSettings{InstallDirectory: root}, root, nil, authConfig{})
	status := app.GetGameStatus()
	if !status.Installed || status.Version != "v0.0.21" {
		t.Fatalf("status = %+v", status)
	}
	if err := app.PlayOffline(); err != nil {
		t.Fatal(err)
	}
	if startedPath != executable || !reflect.DeepEqual(startedArguments, []string{"--offline"}) {
		t.Fatalf("started %q %q", startedPath, startedArguments)
	}
}

func TestNearestExistingDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "missing", "game")
	if got := nearestExistingDirectory(nested); got != root {
		t.Fatalf("nearestExistingDirectory() = %q, want %q", got, root)
	}
	if got := nearestExistingDirectory(root); got != root {
		t.Fatalf("existing directory changed to %q", got)
	}
}
