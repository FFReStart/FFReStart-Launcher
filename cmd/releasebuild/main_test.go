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
