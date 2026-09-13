package patch

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/FFReStart/FFReStart-Launcher/internal/update"
	"golang.org/x/mod/semver"
)

type versionRecord struct {
	Version string `json:"version"`
}

type Installer struct {
	Root        string
	KeyID       string
	PublicKey   ed25519.PublicKey
	Downloader  update.Downloader
	HealthCheck func(context.Context, string) error
	Release     bool
}

func DefaultGameRoot() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "FFReStart", "game"), nil
}

func (i Installer) Install(ctx context.Context, manifestData []byte) error {
	manifest, err := ParseManifestForTarget(manifestData, runtime.GOOS, !i.Release)
	if err != nil {
		return err
	}
	if err := VerifyManifest(manifest, i.KeyID, i.PublicKey); err != nil {
		return err
	}
	current, err := i.currentVersion()
	if err != nil {
		return err
	}
	if current != "" && semver.Compare(manifest.Version, current) <= 0 {
		return ErrNotNewer
	}
	versionsRoot := filepath.Join(i.Root, "versions")
	staging := filepath.Join(versionsRoot, strings.TrimPrefix(manifest.Version, "v")+".staging")
	installed := filepath.Join(versionsRoot, strings.TrimPrefix(manifest.Version, "v"))
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return err
	}
	for _, file := range manifest.Files {
		destination := filepath.Join(staging, filepath.Clean(filepath.FromSlash(file.Path)))
		if err := i.Downloader.Download(ctx, file.URL, destination, file.Size, file.SHA256); err != nil {
			return fmt.Errorf("download game file %q: %w", file.Path, err)
		}
	}
	if err := os.Rename(staging, installed); err != nil {
		return fmt.Errorf("promote game files: %w", err)
	}
	if current != "" {
		if err := writeVersion(filepath.Join(i.Root, "previous.json"), current); err != nil {
			return err
		}
	}
	if err := writeVersion(filepath.Join(i.Root, "current.json"), manifest.Version); err != nil {
		if current != "" {
			_ = writeVersion(filepath.Join(i.Root, "current.json"), current)
		}
		return err
	}
	if i.HealthCheck != nil {
		healthCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = i.HealthCheck(healthCtx, installed)
		cancel()
		if err != nil {
			return errors.Join(fmt.Errorf("game health check: %w", err), i.Rollback())
		}
	}
	return nil
}

func (i Installer) CurrentPath() (string, error) {
	version, err := i.currentVersion()
	if err != nil || version == "" {
		return "", err
	}
	return filepath.Join(i.Root, "versions", strings.TrimPrefix(version, "v")), nil
}

func (i Installer) Rollback() error {
	previous, err := readVersion(filepath.Join(i.Root, "previous.json"))
	if err != nil {
		return fmt.Errorf("read previous game version: %w", err)
	}
	if _, err := os.Stat(filepath.Join(i.Root, "versions", strings.TrimPrefix(previous, "v"))); err != nil {
		return fmt.Errorf("previous game version is unavailable: %w", err)
	}
	return writeVersion(filepath.Join(i.Root, "current.json"), previous)
}

func (i Installer) currentVersion() (string, error) {
	version, err := readVersion(filepath.Join(i.Root, "current.json"))
	if os.IsNotExist(err) {
		return "", nil
	}
	return version, err
}

func readVersion(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is within application data.
	if err != nil {
		return "", err
	}
	var record versionRecord
	if json.Unmarshal(data, &record) != nil || !semver.IsValid(record.Version) {
		return "", errors.New("invalid installed game version record")
	}
	return record.Version, nil
}

func writeVersion(path, version string) error {
	if !semver.IsValid(version) {
		return errors.New("invalid installed game version")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(versionRecord{Version: version})
	if err != nil {
		return err
	}
	return update.AtomicWriteFile(path, data, 0o600)
}
