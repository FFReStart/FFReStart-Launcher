package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func folderOpenCommand(goos, path string) (string, []string, error) {
	switch goos {
	case "windows":
		return "explorer.exe", []string{path}, nil
	case "darwin":
		return "open", []string{path}, nil
	case "linux":
		return "xdg-open", []string{path}, nil
	default:
		return "", nil, fmt.Errorf("opening folders is unsupported on %s", goos)
	}
}

func urlOpenCommand(goos, target string) (string, []string, error) {
	switch goos {
	case "windows":
		return "explorer.exe", []string{target}, nil
	case "darwin":
		return "open", []string{target}, nil
	case "linux":
		return "xdg-open", []string{target}, nil
	default:
		return "", nil, fmt.Errorf("opening links is unsupported on %s", goos)
	}
}

func openInstallFolder(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create game folder: %w", err)
	}
	command, arguments, err := folderOpenCommand(runtime.GOOS, path)
	if err != nil {
		return err
	}
	// #nosec G204 -- executable is selected from a closed OS switch and the
	// path is passed as one explicit argument without a shell.
	if err := exec.Command(command, arguments...).Start(); err != nil {
		return fmt.Errorf("open game folder: %w", err)
	}
	return nil
}

func openExternalURL(target string) error {
	if target == "" {
		return errors.New("link is unavailable")
	}
	command, arguments, err := urlOpenCommand(runtime.GOOS, target)
	if err != nil {
		return err
	}
	// #nosec G204 -- executable is selected from a closed OS switch and the
	// compile-time URL is passed as one explicit argument without a shell.
	if err := exec.Command(command, arguments...).Start(); err != nil {
		return fmt.Errorf("open link: %w", err)
	}
	return nil
}
