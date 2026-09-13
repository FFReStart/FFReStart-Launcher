//go:build !windows

package launch

import (
	"os/exec"
	"testing"
)

func TestDetachedUnixProcessConfiguration(t *testing.T) {
	t.Parallel()
	command := exec.Command("game")
	configureDetachedProcess(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.Setsid {
		t.Fatal("game process does not start in a detached session")
	}
}
