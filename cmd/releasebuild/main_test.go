package main

import (
	"errors"
	"strings"
	"testing"

	launcherupdate "github.com/FFReStart/FFReStart-Launcher/internal/update"
)

func TestReleaseToolRejectsKnownTestKeyUnderProductionID(t *testing.T) {
	const deterministicTestPublicKey = "79b5562e8fe654f94078b112e8a98ba7901f853ae695bed7e0e3910bad049664"
	if err := validateReleaseKeys("release-aliased-test-key", deterministicTestPublicKey, "game-release", strings.Repeat("1", 64)); !errors.Is(err, launcherupdate.ErrTestReleaseKey) {
		t.Fatalf("error = %v, want known test key rejection", err)
	}
}

func TestReleaseToolRequiresProductionServerMetadata(t *testing.T) {
	t.Parallel()
	valid := []string{"https://auth.example.test", "https://api.example.test", "launcher-client", "device-client", "project-id", "7", strings.Repeat("a", 64)}
	if err := validateReleaseServer(valid[0], valid[1], valid[2], valid[3], valid[4], valid[5], valid[6]); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func([]string){
		"http issuer":   func(values []string) { values[0] = "http://auth.example.test" },
		"missing api":   func(values []string) { values[1] = "" },
		"bad client":    func(values []string) { values[2] = "client id" },
		"zero protocol": func(values []string) { values[5] = "0" },
		"bad hash":      func(values []string) { values[6] = strings.Repeat("z", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			values := append([]string(nil), valid...)
			mutate(values)
			if err := validateReleaseServer(values[0], values[1], values[2], values[3], values[4], values[5], values[6]); err == nil {
				t.Fatal("invalid production metadata was accepted")
			}
		})
	}
}
