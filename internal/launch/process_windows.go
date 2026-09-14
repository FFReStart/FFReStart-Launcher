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

// configureDetachedStdinProcess is configureDetachedProcess for a game that
// reads the launch hand-off from stdin. NoInheritHandles would keep the pipe's
// read end from the game, so writing the hand-off failed with "The pipe is
// being closed". Go lets the child inherit only its standard handles
// (PROC_THREAD_ATTRIBUTE_HANDLE_LIST), so no other launcher handle leaks.
func configureDetachedStdinProcess(command *exec.Cmd) {
	configureDetachedProcess(command)
	command.SysProcAttr.NoInheritHandles = false
}
