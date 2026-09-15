package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testServerConfiguration(baseURL string) ServerConfiguration {
	return ServerConfiguration{
		Issuer:            baseURL,
		ControlAPIBaseURL: baseURL,
		BootstrapURL:      baseURL + "/v1/bootstrap",
		LaunchJWKSURL:     baseURL + "/.well-known/jwks.json",
		ProtocolVersion:   1,
		BuildHash:         strings.Repeat("a", 64),
		LauncherClientID:  "launcher-client",
		DeviceClientID:    "device-client",
		AudienceProjectID: "project-id",
		Scopes:            "openid offline_access audience",
	}
}

func TestServerConfigurationURLPolicy(t *testing.T) {
	t.Parallel()
	for name, config := range map[string]ServerConfiguration{
		"loopback http":  testServerConfiguration("http://127.0.0.1:3000"),
		"localhost http": testServerConfiguration("http://localhost:3000"),
		"public https":   testServerConfiguration("https://api.example.test"),
	} {
		config := config
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := validateServerConfiguration(config); err != nil {
				t.Fatal(err)
			}
		})
	}
	// #nosec G101 -- deliberately invalid public URL fixtures, not credentials.
	for name, raw := range map[string]string{
		"public http": "http://192.0.2.10:3000",
		"credentials": "https://user:password@example.test",
		"query":       "https://example.test?redirect=attacker",
	} {
		t.Run(name, func(t *testing.T) {
			config := testServerConfiguration(raw)
			if _, err := validateServerConfiguration(config); err == nil {
				t.Fatalf("accepted unsafe server URL %q", raw)
			}
		})
	}
}

func TestLoadGeneratedDeveloperServerConfiguration(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "dev-launcher-config.json")
	data, err := json.Marshal(testServerConfiguration("http://127.0.0.1:3000"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := loadServerConfiguration(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.DeviceClientID != "device-client" || !strings.Contains(config.Scopes, "offline_access") {
		t.Fatalf("unexpected imported configuration: %+v", config)
	}
}

func TestSettingsPersistServerMetadataWithoutCredentials(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "settings.json")
	store := &settingsStore{path: path}
	settings := defaultSettings(t.TempDir())
	settings.Server = testServerConfiguration("https://api.example.test")
	settings.AuthFlow = "device"
	settings.SelectedRealm = "local"
	if err := store.Save(settings); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- test-owned temporary settings path.
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"access-secret", "refresh-secret", "ticket-secret"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("settings leaked %s", forbidden)
		}
	}
	if got := store.Load("unused"); got.Server != settings.Server || got.SelectedRealm != "local" {
		t.Fatalf("server metadata did not round trip: %+v", got)
	}
}
