package handoff

import (
	"bytes"
	"fmt"
	"os/exec"

	"google.golang.org/protobuf/encoding/protodelim"
	launchv1 "wails-spike/proto/launch/v1"
)

// LaunchGame places only the mode flag on argv and writes exactly one
// length-delimited protobuf message to the child's inherited stdin pipe.
func LaunchGame(path string, message *launchv1.LaunchHandoff) (string, error) {
	cmd := exec.Command(path, "--auth-token-stdin")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", err
	}
	var stdout bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stdout
	if err := cmd.Start(); err != nil {
		return "", err
	}
	if _, err := protodelim.MarshalTo(stdin, message); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return "", err
	}
	if err := stdin.Close(); err != nil {
		return "", err
	}
	if err := cmd.Wait(); err != nil {
		return stdout.String(), fmt.Errorf("game failed: %w: %s", err, stdout.String())
	}
	return stdout.String(), nil
}
