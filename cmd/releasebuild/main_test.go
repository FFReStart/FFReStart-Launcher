package main

import (
	"errors"
	"testing"

	launcherupdate "github.com/FFReStart/FFReStart-Launcher/internal/update"
)

func TestReleaseToolRejectsKnownTestKeyUnderProductionID(t *testing.T) {
	const deterministicTestPublicKey = "79b5562e8fe654f94078b112e8a98ba7901f853ae695bed7e0e3910bad049664"
	if err := validateReleaseKey("release-aliased-test-key", deterministicTestPublicKey); !errors.Is(err, launcherupdate.ErrTestReleaseKey) {
		t.Fatalf("error = %v, want known test key rejection", err)
	}
}
