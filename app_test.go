package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
)

type appCaptureStarter struct {
	args    []string
	payload []byte
}

func (s *appCaptureStarter) Start(context.Context, string, ...string) (launch.Process, error) {
	return appTestProcess{}, nil
}

func (s *appCaptureStarter) StartWithStdin(_ context.Context, _ string, payload []byte, args ...string) (launch.Process, error) {
	s.args = append([]string(nil), args...)
	s.payload = append([]byte(nil), payload...)
	return appTestProcess{}, nil
}

type panicTokenStore struct{ calls atomic.Int32 }

func (s *panicTokenStore) LoadRefresh(context.Context) (string, error) {
	s.calls.Add(1)
	panic("offline launch read a token")
}

func TestInstalledGamePlayDoesNotWaitForInstallAction(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(requestStarted)
		<-releaseRequest
	}))
	defer server.Close()

	launcher := launch.NewService("game", launch.FuncStarter(func(context.Context, string, ...string) (launch.Process, error) { return appTestProcess{}, nil }), nil)
	launcher.SetLaunchGrace(time.Millisecond)
	app := NewApp(launcher, nil)
	app.startup(context.Background())
	app.ConfigureInstaller(&patch.Installer{Root: t.TempDir()}, server.URL, server.Client())
	installDone := make(chan error, 1)
	go func() { installDone <- app.InstallOrUpdate() }()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("install request did not start")
	}
	begin := time.Now()
	if err := app.PlayOffline(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(begin); elapsed > 100*time.Millisecond {
		t.Fatalf("offline play waited %s for install action", elapsed)
	}
	close(releaseRequest)
	<-installDone
}
func (s *panicTokenStore) SaveRefresh(context.Context, string) error {
	s.calls.Add(1)
	panic("offline launch created a token")
}

func TestOfflineAppHasNoTokenPath(t *testing.T) {
	store := &panicTokenStore{}
	launcher := launch.NewService("game", launch.FuncStarter(func(context.Context, string, ...string) (launch.Process, error) { return appTestProcess{}, nil }), nil)
	launcher.SetLaunchGrace(time.Millisecond)
	app := NewApp(launcher, store)
	app.startup(context.Background())
	if err := app.PlayOffline(); err != nil {
		t.Fatal(err)
	}
	if got := store.calls.Load(); got != 0 {
		t.Fatalf("offline launch touched token storage %d times", got)
	}
}

type appTestProcess struct{}

func (appTestProcess) Wait() error {
	time.Sleep(time.Second)
	return nil
}

func TestSignedManifestDisablesDeveloperChannel(t *testing.T) {
	t.Parallel()
	app := NewApp(nil, nil)
	app.ConfigureInstaller(&patch.Installer{Root: t.TempDir()}, "https://updates.example/game.json", nil)
	app.configureDeveloperInstaller(&patch.DeveloperInstaller{Root: t.TempDir()})
	state := app.GetLauncherState()
	if state.DeveloperChannel || state.UpdateChannel != "SIGNED GAME MANIFEST" {
		t.Fatalf("signed manifest did not take precedence: %+v", state)
	}
}

func TestSuccessfulLaunchClosesLauncher(t *testing.T) {
	t.Parallel()
	launcher := launch.NewService("game", launch.FuncStarter(func(context.Context, string, ...string) (launch.Process, error) {
		return appTestProcess{}, nil
	}), nil)
	launcher.SetLaunchGrace(time.Millisecond)
	app := NewApp(launcher, nil)
	closed := false
	app.configureQuit(func() { closed = true })
	if err := app.PlayOffline(); err != nil {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("launcher remained open after a successful launch")
	}
}

func TestFailedLaunchKeepsLauncherOpen(t *testing.T) {
	t.Parallel()
	want := errors.New("start failure")
	launcher := launch.NewService("game", launch.FuncStarter(func(context.Context, string, ...string) (launch.Process, error) {
		return nil, want
	}), nil)
	app := NewApp(launcher, nil)
	closed := false
	app.configureQuit(func() { closed = true })
	if err := app.PlayOffline(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want start failure", err)
	}
	if closed {
		t.Fatal("launcher closed after a failed launch")
	}
}

// After a sign-out everywhere the launcher's refresh token still refreshes at
// the identity provider, but the control API refuses every token of the old
// sign-in. The launcher must return to signed out instead of showing a signed-in
// state whose every PLAY MULTIPLAYER fails.
func TestRefusedSignInReturnsToSignedOut(t *testing.T) {
	var revoked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/oauth/v2/device_authorization":
			_ = json.NewEncoder(writer).Encode(map[string]any{"device_code": "device-secret", "user_code": "FUSION", "verification_uri": "https://example.test/device", "interval": 1})
		case "/oauth/v2/token":
			_ = json.NewEncoder(writer).Encode(map[string]string{"access_token": "access-secret", "refresh_token": "refresh-secret"})
		case "/v1/me", "/v1/launch-tickets":
			if revoked.Load() {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"id": "account-uuid", "status": "active"})
		case "/v1/bootstrap":
			_ = json.NewEncoder(writer).Encode(map[string]any{"realms": []map[string]any{{"realmId": "local", "name": "Local", "status": "online", "admissionOpen": true, "worldEndpoint": "127.0.0.1:27020", "gnsCaKeyId": "dev-ca", "protocolMin": 1, "protocolMax": 1, "contentVersion": "dev-content", "updateChannel": "development"}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	starter := &appCaptureStarter{}
	launcher := launch.NewService("game.exe", starter, nil)
	launcher.SetLaunchGrace(time.Millisecond)
	refreshStore := &auth.MemoryStore{}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settings := defaultSettings(t.TempDir())
	settings.Server = testServerConfiguration(server.URL)
	app := NewApp(launcher, nil)
	app.ConfigureExperience(&settingsStore{path: settingsPath}, settings, settings.InstallDirectory, refreshStore)
	app.ConfigureInstaller(nil, "", server.Client())
	app.ConfigureIdentityClient(server.Client())
	app.ConfigureMultiplayerBuild(1, strings.Repeat("a", 64))
	app.devicePoll = time.Millisecond
	app.devicePrompt = func(auth.DevicePrompt) error { return nil }
	app.startup(context.Background())
	if err := app.SignInWithCode(true); err != nil {
		t.Fatal(err)
	}
	revoked.Store(true)
	if err := app.PlayMultiplayer(); !errors.Is(err, errSignInEnded) {
		t.Fatalf("PlayMultiplayer after the server refused the sign-in = %v, want %v", err, errSignInEnded)
	}
	if starter.payload != nil {
		t.Fatal("the game started without a ticket")
	}
	if state := app.GetLauncherState(); state.SignedIn || state.AccountID != "" {
		t.Fatalf("the launcher still shows the refused sign-in: %+v", state)
	}
	if _, err := refreshStore.Load(); err == nil {
		t.Fatal("the refused sign-in's refresh token was kept")
	}
	if saved := (&settingsStore{path: settingsPath}).Load(t.TempDir()); saved.AuthFlow != "" {
		t.Fatalf("the next start would retry the refused sign-in (auth flow %q)", saved.AuthFlow)
	}
}

func TestDeviceSignInRefreshTicketAndHandoff(t *testing.T) {
	const (
		accessOne = "access-secret-one"
		accessTwo = "access-secret-two"
		refresh   = "refresh-secret"
		ticket    = "ticket-secret"
	)
	var ticketCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/oauth/v2/device_authorization":
			_ = json.NewEncoder(writer).Encode(map[string]any{"device_code": "device-secret", "user_code": "FUSION", "verification_uri": "https://example.test/device", "interval": 1})
		case "/oauth/v2/token":
			_ = request.ParseForm()
			if request.Form.Get("grant_type") == "refresh_token" {
				if request.Form.Get("refresh_token") != refresh || request.Form.Get("client_id") != "device-client" {
					t.Error("refresh request used the wrong credential metadata")
				}
				_ = json.NewEncoder(writer).Encode(map[string]string{"access_token": accessTwo, "refresh_token": refresh})
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"access_token": accessOne, "refresh_token": refresh})
		case "/v1/me":
			authorization := request.Header.Get("Authorization")
			if authorization != "Bearer "+accessOne && authorization != "Bearer "+accessTwo {
				t.Errorf("unexpected /v1/me authorization %q", authorization)
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"id": "account-uuid", "status": "active", "profile": map[string]string{"handle": "FusionPilot"}})
		case "/v1/bootstrap":
			_ = json.NewEncoder(writer).Encode(map[string]any{"realms": []map[string]any{{"realmId": "local", "name": "Local", "status": "online", "admissionOpen": true, "worldEndpoint": "127.0.0.1:27020", "gnsCaKeyId": "dev-ca", "protocolMin": 1, "protocolMax": 1, "contentVersion": "dev-content", "updateChannel": "development"}}})
		case "/v1/launch-tickets":
			if ticketCalls.Add(1) == 1 {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			if request.Header.Get("Authorization") != "Bearer "+accessTwo {
				t.Error("ticket request did not retry with the refreshed access token")
			}
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"ticket": ticket, "expiresAt": time.Now().Add(time.Minute).UTC(),
				"bootstrap": map[string]any{"realmId": "local", "worldEndpoint": "127.0.0.1:27020", "gnsCaKeyId": "dev-ca", "protocolMin": 1, "protocolMax": 1, "contentVersion": "dev-content"},
			})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	starter := &appCaptureStarter{}
	launcher := launch.NewService("game.exe", starter, nil)
	launcher.SetLaunchGrace(time.Millisecond)
	refreshStore := &auth.MemoryStore{}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settings := defaultSettings(t.TempDir())
	settings.Server = testServerConfiguration(server.URL)
	store := &settingsStore{path: settingsPath}
	app := NewApp(launcher, nil)
	app.ConfigureExperience(store, settings, settings.InstallDirectory, refreshStore)
	app.ConfigureInstaller(nil, "", server.Client())
	app.ConfigureIdentityClient(server.Client())
	app.ConfigureMultiplayerBuild(1, strings.Repeat("a", 64))
	app.devicePoll = time.Millisecond
	app.devicePrompt = func(prompt auth.DevicePrompt) error {
		if prompt.UserCode != "FUSION" {
			t.Errorf("device prompt = %+v", prompt)
		}
		return nil
	}
	app.startup(context.Background())

	var logs bytes.Buffer
	previousLogWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousLogWriter)
	if err := app.SignInWithCode(true); err != nil {
		t.Fatal(err)
	}
	state := app.GetLauncherState()
	if !state.SignedIn || state.AccountHandle != "FusionPilot" || state.AccountID != "account-uuid" || state.SelectedRealm != "local" {
		t.Fatalf("unexpected signed-in state: %+v", state)
	}
	if err := app.PlayMultiplayer(); err != nil {
		t.Fatal(err)
	}
	joinedArgs := strings.Join(starter.args, " ")
	if joinedArgs != "--auth-token-stdin" || strings.Contains(joinedArgs, ticket) || strings.Contains(joinedArgs, accessOne) || strings.Contains(joinedArgs, accessTwo) {
		t.Fatalf("credential reached argv: %q", joinedArgs)
	}
	if !bytes.Contains(starter.payload, []byte(ticket)) || bytes.Contains(starter.payload, []byte(accessOne)) || bytes.Contains(starter.payload, []byte(accessTwo)) {
		t.Fatal("stdin hand-off did not contain only the launch credential")
	}
	settingsData, err := os.ReadFile(settingsPath) // #nosec G304 -- test-owned temporary settings path.
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{accessOne, accessTwo, refresh, ticket} {
		if bytes.Contains(settingsData, []byte(secret)) || strings.Contains(logs.String(), secret) {
			t.Fatalf("credential %q reached settings or logs", secret)
		}
	}
	if err := app.SignOut(); err != nil {
		t.Fatal(err)
	}
	if _, err := refreshStore.Load(); err == nil {
		t.Fatal("sign-out left the refresh token in the keyring abstraction")
	}
}
