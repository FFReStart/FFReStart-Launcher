//go:build windows

package updater

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	selfapply "github.com/creativeprojects/go-selfupdate/update"
)

func TestWindowsReplacesExecutableWhileItIsRunning(t *testing.T) {
	target := filepath.Join(t.TempDir(), "running-launcher.exe")
	build := exec.Command("go", "build", "-o", target, "./cmd/update-victim")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build victim: %v: %s", err, out)
	}
	proc := exec.Command(target)
	stdout, err := proc.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = proc.Process.Kill(); _ = proc.Wait() }()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("victim not running: %q %v", line, err)
	}
	replacement := []byte("verified replacement bytes")
	if err := selfapply.Apply(bytes.NewReader(replacement), selfapply.Options{TargetPath: target}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, replacement) {
		t.Fatalf("running exe was not replaced: %v", err)
	}
}
