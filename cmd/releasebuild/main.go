package main

import (
	"fmt"
	"os"
	"os/exec"

	launcherupdate "github.com/FFReStart/FFReStart-Launcher/internal/update"
)

func main() {
	platform := "windows/amd64"
	if len(os.Args) > 1 {
		platform = os.Args[1]
	}
	keyID := os.Getenv("FFRESTART_UPDATE_KEY_ID")
	publicKey := os.Getenv("FFRESTART_UPDATE_PUBLIC_KEY_HEX")
	gameKeyID := os.Getenv("FFRESTART_GAME_KEY_ID")
	gamePublicKey := os.Getenv("FFRESTART_GAME_PUBLIC_KEY_HEX")
	if err := validateReleaseKeys(keyID, publicKey, gameKeyID, gamePublicKey); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	linkerFlags := fmt.Sprintf("-X main.releaseMode=true -X main.updateKeyID=%s -X main.updatePublicKeyHex=%s -X main.gameKeyID=%s -X main.gamePublicKeyHex=%s", keyID, publicKey, gameKeyID, gamePublicKey)
	// #nosec G204,G702 -- validated release parameters are forwarded to the pinned Wails CLI.
	command := exec.Command("wails", "build", "-platform", platform, "-clean", "-ldflags", linkerFlags)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func validateReleaseKeys(keyID, publicKey, gameKeyID, gamePublicKey string) error {
	if _, err := launcherupdate.ReleasePublicKey(keyID, publicKey, true); err != nil {
		return fmt.Errorf("launcher update key: %w", err)
	}
	if _, err := launcherupdate.ReleasePublicKey(gameKeyID, gamePublicKey, true); err != nil {
		return fmt.Errorf("game manifest key: %w", err)
	}
	return nil
}
