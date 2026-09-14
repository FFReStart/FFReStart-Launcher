package launch

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const stdinHelperOutput = "FFRESTART_STDIN_HELPER_OUTPUT"

// The real starter must hand the game its stdin pipe. On Windows, starting the
// game with NoInheritHandles made every hand-off write fail with "The pipe is
// being closed"; the fake starters in the other tests cannot catch that.
func TestExecStarterDeliversStdinHandOff(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "stdin.bin")
	t.Setenv(stdinHelperOutput, output)
	payload := []byte("launch hand-off\x00\x01\x02")
	process, err := ExecStarter{}.StartWithStdin(context.Background(), executable, payload, "-test.run=^TestExecStarterStdinHelper$")
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Wait(); err != nil {
		t.Fatalf("stdin helper failed: %v", err)
	}
	received, err := os.ReadFile(output) // #nosec G304 -- the test's own temporary file.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(received, payload) {
		t.Fatalf("the child read %q from stdin, want %q", received, payload)
	}
}

// TestExecStarterStdinHelper is the child process of
// TestExecStarterDeliversStdinHandOff: it copies stdin to a file.
func TestExecStarterStdinHelper(t *testing.T) {
	output := os.Getenv(stdinHelperOutput)
	if output == "" {
		return
	}
	received, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(output, received, 0o600); err != nil { // #nosec G703 -- the parent test's own temporary file.
		os.Exit(3)
	}
	os.Exit(0)
}
