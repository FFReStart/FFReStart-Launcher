//go:build windows

package launch

import (
	"errors"
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

// breakawayRefused reports a start that Windows refused, which for a detached
// start means the launcher runs in a job that does not allow breakaway.
func breakawayRefused(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
}

// stayInJob keeps every other detached setting and drops only the breakaway.
func stayInJob(command *exec.Cmd) {
	command.SysProcAttr.CreationFlags &^= createBreakawayFromJob
}
