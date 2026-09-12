package handoff

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	launchv1 "wails-spike/proto/launch/v1"
)

func TestLaunchHandoffIsLengthDelimitedOnStdin(t *testing.T) {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	dummy := filepath.Join(t.TempDir(), "dummy-game"+ext)
	cmd := exec.Command("go", "build", "-o", dummy, "./cmd/dummy-game")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build dummy: %v: %s", err, out)
	}
	const ticket = "secret-launch-ticket-that-must-not-be-in-argv"
	out, err := LaunchGame(dummy, &launchv1.LaunchHandoff{Ticket: ticket, Bootstrap: &launchv1.LaunchBootstrap{RealmId: "academy", WorldEndpoint: "127.0.0.1:23000", GnsCaKeyId: "ca-current", ProtocolMin: 1, ProtocolMax: 2, ContentVersion: "v1"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != "academy" || strings.Contains(out, ticket) {
		t.Fatalf("unexpected child output %q", out)
	}
}
