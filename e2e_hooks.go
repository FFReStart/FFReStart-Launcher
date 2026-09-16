package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
)

// Development test hooks for the automated launcher end-to-end check in
// ReStart-HTTP-Server (`just dev-e2e-check --launcher`). They are not a product
// feature. A release build (releaseMode "true") never reads these variables; a
// development build reads them only when FFRESTART_E2E_HOOKS is exactly 1:
//
//   - refresh tokens stay in process memory, so the OS keyring entry of the
//     developer's own sign-in is never read, written or cleared;
//   - each URL the launcher would open in the system browser (sign-in and the
//     device code page) is written to stdout as one JSON line, {"openUrl": URL},
//     instead;
//   - FFRESTART_E2E_GAME_ARGS, a JSON array, adds development-client automation
//     arguments after --auth-token-stdin on multiplayer launches. Only the flags
//     in e2eGameFlags and --ffr-e2e-* flags are accepted. The ticket still goes
//     only to stdin.
const (
	e2eHooksVariable    = "FFRESTART_E2E_HOOKS"
	e2eGameArgsVariable = "FFRESTART_E2E_GAME_ARGS"
	maxE2EGameArguments = 32
	maxE2EArgumentBytes = 1024
)

// Unity player flags the check may pass, and whether each takes a value.
var e2eGameFlags = map[string]bool{
	"-batchmode":         false,
	"-nographics":        false,
	"-logFile":           true,
	"-screen-fullscreen": true,
	"-screen-width":      true,
	"-screen-height":     true,
}

// Every development-client automation flag takes one value.
var e2eAutomationFlag = regexp.MustCompile(`^--ffr-e2e-[a-z]+(-[a-z]+)*$`)

type e2eHooks struct {
	enabled  bool
	gameArgs []string
	mu       *sync.Mutex
	stdout   io.Writer
}

func loadE2EHooks(release bool, getenv func(string) string, stdout io.Writer) (e2eHooks, error) {
	if release || getenv(e2eHooksVariable) != "1" {
		return e2eHooks{}, nil
	}
	args, err := parseE2EGameArgs(getenv(e2eGameArgsVariable))
	if err != nil {
		return e2eHooks{}, err
	}
	return e2eHooks{enabled: true, gameArgs: args, mu: &sync.Mutex{}, stdout: stdout}, nil
}

func parseE2EGameArgs(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(value), &args); err != nil {
		return nil, fmt.Errorf("%s must be a JSON array of strings: %w", e2eGameArgsVariable, err)
	}
	if len(args) > maxE2EGameArguments {
		return nil, fmt.Errorf("%s allows at most %d arguments", e2eGameArgsVariable, maxE2EGameArguments)
	}
	for index := 0; index < len(args); index++ {
		flag := args[index]
		takesValue, known := e2eGameFlags[flag]
		if !known && e2eAutomationFlag.MatchString(flag) {
			known, takesValue = true, true
		}
		if !known {
			return nil, fmt.Errorf("%s does not allow %q", e2eGameArgsVariable, flag)
		}
		if !takesValue {
			continue
		}
		index++
		if index == len(args) || args[index] == "" || len(args[index]) > maxE2EArgumentBytes {
			return nil, fmt.Errorf("%s needs a value of at most %d bytes after %s", e2eGameArgsVariable, maxE2EArgumentBytes, flag)
		}
	}
	return args, nil
}

// refreshStore keeps the hooked launcher's refresh token in memory only.
func (h e2eHooks) refreshStore(defaultStore *auth.FallbackStore) *auth.FallbackStore {
	if !h.enabled {
		return defaultStore
	}
	return &auth.FallbackStore{Primary: &auth.MemoryStore{}, Memory: &auth.MemoryStore{}}
}

// openURL reports a URL the launcher would open in the system browser.
func (h e2eHooks) openURL(location string) error {
	if !h.enabled {
		return errors.New("the e2e test hooks are disabled")
	}
	line, err := json.Marshal(struct {
		OpenURL string `json:"openUrl"`
	}{location})
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err = h.stdout.Write(append(line, '\n'))
	return err
}
