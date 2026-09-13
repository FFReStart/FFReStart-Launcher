//go:build windows

package launch

import (
	"os/exec"
	"syscall"
)

const (
	detachedProcess        = 0x00000008
	createNewProcessGroup  = 0x00000200
	createBreakawayFromJob = 0x01000000
)

func configureDetachedProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:    detachedProcess | createNewProcessGroup | createBreakawayFromJob,
		NoInheritHandles: true,
	}
}
