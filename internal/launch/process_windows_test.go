//go:build windows

package launch

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
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

func TestStayingInTheJobDropsOnlyTheBreakaway(t *testing.T) {
	t.Parallel()
	command := exec.Command("game.exe")
	configureDetachedStdinProcess(command)
	stayInJob(command)
	if want := uint32(detachedProcess | createNewProcessGroup); command.SysProcAttr.CreationFlags != want {
		t.Fatalf("creation flags = %#x, want %#x", command.SysProcAttr.CreationFlags, want)
	}
	if command.SysProcAttr.NoInheritHandles {
		t.Fatal("the stdin hand-off lost its pipe")
	}
	if !breakawayRefused(&os.PathError{Op: "fork/exec", Path: "game.exe", Err: syscall.ERROR_ACCESS_DENIED}) {
		t.Fatal("access denied was not treated as a refused breakaway")
	}
	if breakawayRefused(&os.PathError{Op: "fork/exec", Path: "game.exe", Err: syscall.ERROR_FILE_NOT_FOUND}) {
		t.Fatal("a missing game was retried")
	}
}

const jobHelperOutput = "FFRESTART_JOB_HELPER_OUTPUT"

// A launcher started inside a job that forbids breakaway got "Access is denied"
// for every detached launch. The helper below runs the real starter from such
// a job and must still deliver the hand-off.
func TestExecStarterStartsInsideAJobWithoutBreakaway(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = windows.CloseHandle(job) }()
	output := filepath.Join(t.TempDir(), "stdin.bin")
	helper := exec.Command(executable, "-test.run=^TestJobHelper$") // #nosec G204 -- the current test binary.
	helper.Env = append(os.Environ(), jobHelperOutput+"="+output)
	gate, err := helper.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	// #nosec G115 -- a process ID fits in a DWORD.
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(helper.Process.Pid))
	if err == nil {
		err = windows.AssignProcessToJobObject(job, handle)
		_ = windows.CloseHandle(handle)
	}
	if err != nil {
		_ = helper.Process.Kill()
		_ = helper.Wait()
		t.Fatal(err)
	}
	_ = gate.Close()
	if err := helper.Wait(); err != nil {
		t.Fatalf("the starter failed inside a job without breakaway: %v", err)
	}
	received, err := os.ReadFile(output) // #nosec G304 -- the test's own temporary file.
	if err != nil || !bytes.Equal(received, []byte("hand-off from a job")) {
		t.Fatalf("the game read %q, %v", received, err)
	}
}

// TestJobHelper is the launcher of TestExecStarterStartsInsideAJobWithoutBreakaway:
// once its parent has put it in the job, it starts TestExecStarterStdinHelper.
func TestJobHelper(t *testing.T) {
	output := os.Getenv(jobHelperOutput)
	if output == "" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	executable, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	if err := os.Setenv(stdinHelperOutput, output); err != nil {
		os.Exit(3)
	}
	process, err := ExecStarter{}.StartWithStdin(context.Background(), executable, []byte("hand-off from a job"), "-test.run=^TestExecStarterStdinHelper$")
	if err != nil {
		os.Exit(4)
	}
	if err := process.Wait(); err != nil {
		os.Exit(5)
	}
	os.Exit(0)
}
