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
