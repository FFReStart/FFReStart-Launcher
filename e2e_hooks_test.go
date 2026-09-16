package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
)

func e2eEnvironment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestE2EHooksAreIgnoredInReleaseBuilds(t *testing.T) {
	read := false
	hooks, err := loadE2EHooks(true, func(string) string { read = true; return "1" }, &bytes.Buffer{})
	if err != nil || hooks.enabled || hooks.gameArgs != nil || read {
		t.Fatalf("release build used the hooks: %+v, %v, read environment %v", hooks, err, read)
	}
	keyring := &auth.FallbackStore{Primary: auth.KeyringStore{Service: auth.KeyringService, User: "refresh-token"}, Memory: &auth.MemoryStore{}}
	if hooks.refreshStore(keyring) != keyring {
		t.Fatal("release build replaced the keyring store")
	}
}

func TestE2EHooksNeedTheExactSwitch(t *testing.T) {
	for _, value := range []string{"", "0", "true", "yes", " 1"} {
		hooks, err := loadE2EHooks(false, e2eEnvironment(map[string]string{e2eHooksVariable: value, e2eGameArgsVariable: `["--auth-token-stdin"]`}), &bytes.Buffer{})
		if err != nil || hooks.enabled || hooks.gameArgs != nil {
			t.Fatalf("%s=%q enabled the hooks: %+v, %v", e2eHooksVariable, value, hooks, err)
		}
	}
}

func TestE2EHooksKeepRefreshTokensInMemory(t *testing.T) {
	hooks, err := loadE2EHooks(false, e2eEnvironment(map[string]string{e2eHooksVariable: "1"}), &bytes.Buffer{})
	if err != nil || !hooks.enabled {
		t.Fatalf("hooks = %+v, %v", hooks, err)
	}
	keyring := &auth.FallbackStore{Primary: auth.KeyringStore{Service: auth.KeyringService, User: "refresh-token"}, Memory: &auth.MemoryStore{}}
	store := hooks.refreshStore(keyring)
	if store == keyring {
		t.Fatal("the hooked launcher kept the OS keyring store")
	}
	if _, memory := store.Primary.(*auth.MemoryStore); !memory {
		t.Fatalf("primary store is %T, not process memory", store.Primary)
	}
}

func TestE2EHooksAcceptAutomationArguments(t *testing.T) {
	args := []string{"-batchmode", "-nographics", "-logFile", `C:\e2e\player.log`, "--ffr-e2e-result", `C:\e2e\result.json`,
		"--ffr-e2e-create", "Launcher Check", "--ffr-e2e-move-heading", "-90", "-screen-width", "1280"}
	encoded, _ := json.Marshal(args)
	hooks, err := loadE2EHooks(false, e2eEnvironment(map[string]string{e2eHooksVariable: "1", e2eGameArgsVariable: string(encoded)}), &bytes.Buffer{})
	if err != nil || strings.Join(hooks.gameArgs, "\x00") != strings.Join(args, "\x00") {
		t.Fatalf("hooks = %+v, %v", hooks, err)
	}
}

func TestE2EHooksRefuseOtherArguments(t *testing.T) {
	for _, value := range []string{
		`["--auth-token-stdin"]`,
		`["--offline"]`,
		`["--auth-token", "secret"]`,
		`["secret"]`,
		`["--ffr-e2e-result"]`,
		`["--ffr-e2e-result", ""]`,
		`["--ffr-e2e-Result", "x"]`,
		`["-logFile"]`,
		`"-batchmode"`,
		`[1]`,
		`["-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode", "-batchmode"]`,
		`["--ffr-e2e-create", "` + strings.Repeat("x", maxE2EArgumentBytes+1) + `"]`,
	} {
		if hooks, err := loadE2EHooks(false, e2eEnvironment(map[string]string{e2eHooksVariable: "1", e2eGameArgsVariable: value}), &bytes.Buffer{}); err == nil {
			t.Fatalf("%s accepted %s: %+v", e2eGameArgsVariable, value, hooks)
		}
	}
}

func TestE2EHooksReportBrowserURLsOnStdout(t *testing.T) {
	var stdout bytes.Buffer
	hooks, err := loadE2EHooks(false, e2eEnvironment(map[string]string{e2eHooksVariable: "1"}), &stdout)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"http://auth.example.test/oauth/v2/authorize?state=s", "http://auth.example.test/device?user_code=ABCD"}
	for _, location := range want {
		if err := hooks.openURL(location); err != nil {
			t.Fatal(err)
		}
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != len(want) {
		t.Fatalf("stdout = %q", stdout.String())
	}
	for index, line := range lines {
		var event struct {
			OpenURL string `json:"openUrl"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil || event.OpenURL != want[index] {
			t.Fatalf("line %d = %q", index, line)
		}
	}
}
