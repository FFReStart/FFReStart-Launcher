package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

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
	issuer := os.Getenv("FFRESTART_ZITADEL_ISSUER")
	controlAPI := os.Getenv("FFRESTART_CONTROL_API_BASE_URL")
	launcherClientID := os.Getenv("FFRESTART_ZITADEL_CLIENT_ID")
	deviceClientID := os.Getenv("FFRESTART_ZITADEL_DEVICE_CLIENT_ID")
	audienceProjectID := os.Getenv("FFRESTART_ZITADEL_AUDIENCE_PROJECT_ID")
	protocolVersion := os.Getenv("FFRESTART_PROTOCOL_VERSION")
	buildHash := os.Getenv("FFRESTART_GAME_BUILD_HASH")
	if err := validateReleaseKeys(keyID, publicKey, gameKeyID, gamePublicKey); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := validateReleaseServer(issuer, controlAPI, launcherClientID, deviceClientID, audienceProjectID, protocolVersion, buildHash); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	linkerFlags := fmt.Sprintf("-X main.releaseMode=true -X main.updateKeyID=%s -X main.updatePublicKeyHex=%s -X main.gameKeyID=%s -X main.gamePublicKeyHex=%s -X main.productionIssuer=%s -X main.productionControlAPI=%s -X main.productionLauncherClientID=%s -X main.productionDeviceClientID=%s -X main.productionAudienceProjectID=%s -X main.productionProtocolVersion=%s -X main.productionBuildHash=%s", keyID, publicKey, gameKeyID, gamePublicKey, issuer, controlAPI, launcherClientID, deviceClientID, audienceProjectID, protocolVersion, strings.ToLower(buildHash))
	// #nosec G204,G702 -- validated release parameters are forwarded to the pinned Wails CLI.
	command := exec.Command("wails", "build", "-platform", platform, "-clean", "-ldflags", linkerFlags)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var releaseHash = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

func validateReleaseServer(issuer, controlAPI, launcherClientID, deviceClientID, audienceProjectID, protocolVersion, buildHash string) error {
	for label, raw := range map[string]string{"issuer": issuer, "control API": controlAPI} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("production %s must be an HTTPS URL", label)
		}
	}
	for label, value := range map[string]string{"launcher client ID": launcherClientID, "device client ID": deviceClientID, "audience project ID": audienceProjectID} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, " \t\r\n\"'") {
			return fmt.Errorf("production %s is invalid", label)
		}
	}
	protocol, err := strconv.ParseUint(protocolVersion, 10, 32)
	if err != nil || protocol == 0 {
		return errors.New("production protocol version is invalid")
	}
	if !releaseHash.MatchString(buildHash) {
		return errors.New("production game build hash is invalid")
	}
	return nil
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
