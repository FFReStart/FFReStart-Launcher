//go:build !windows

package launch

import (
	"os/exec"
	"syscall"
)

func configureDetachedProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func configureDetachedStdinProcess(command *exec.Cmd) {
	configureDetachedProcess(command)
}

// Jobs are a Windows concept; a detached start elsewhere is never retried.
func breakawayRefused(error) bool { return false }

func stayInJob(*exec.Cmd) {}
