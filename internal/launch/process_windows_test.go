//go:build windows

package launch

import (
	"os/exec"
	"testing"
)

func TestDetachedWindowsProcessConfiguration(t *testing.T) {
	t.Parallel()
	command := exec.Command("game.exe")
	configureDetachedProcess(command)
	wantFlags := uint32(detachedProcess | createNewProcessGroup | createBreakawayFromJob)
	if command.SysProcAttr == nil || command.SysProcAttr.CreationFlags&wantFlags != wantFlags {
		t.Fatalf("creation flags = %#v, want %#x", command.SysProcAttr, wantFlags)
	}
	if !command.SysProcAttr.NoInheritHandles {
		t.Fatal("game process can inherit launcher handles")
	}
}
