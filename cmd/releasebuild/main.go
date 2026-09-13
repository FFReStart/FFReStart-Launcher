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
	if err := validateReleaseKey(keyID, publicKey); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	linkerFlags := fmt.Sprintf("-X main.releaseMode=true -X main.updateKeyID=%s -X main.updatePublicKeyHex=%s", keyID, publicKey)
	// #nosec G204,G702 -- validated release parameters are forwarded to the pinned Wails CLI.
	command := exec.Command("wails", "build", "-platform", platform, "-clean", "-ldflags", linkerFlags)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func validateReleaseKey(keyID, publicKey string) error {
	_, err := launcherupdate.ReleasePublicKey(keyID, publicKey, true)
	return err
}
