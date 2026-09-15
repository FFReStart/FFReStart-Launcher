package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
	"github.com/FFReStart/FFReStart-Launcher/internal/multiplayer"
	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the narrow JavaScript-to-Go boundary. Token values never appear in a
// bound argument, return value, event, log message, environment or argv.
type App struct {
	ctx            context.Context
	launcher       *launch.Service
	tokens         auth.TokenVault
	refreshStore   auth.RefreshStore
	installer      *patch.Installer
	developer      *patch.DeveloperInstaller
	manifestURL    string
	downloadClient *http.Client
	identityClient *http.Client
	settingsStore  *settingsStore
	settings       LauncherSettings
	defaultRoot    string
	protocol       uint32
	buildHash      string
	quit           func()
	openBrowser    func(string) error
	devicePrompt   func(auth.DevicePrompt) error
	stateChanged   func()
	devicePoll     time.Duration
	authMu         sync.Mutex

	mu                   sync.RWMutex
	status               GameStatus
	signedIn             bool
	accessToken, idToken string
	account              multiplayer.Account
	realms               []multiplayer.Realm
}

type GameStatus struct {
	Installed bool   `json:"installed"`
	Busy      bool   `json:"busy"`
	Message   string `json:"message"`
	Title     string `json:"title"`
	Version   string `json:"version"`
	Progress  int    `json:"progress"`
}

type LauncherState struct {
	Game                    GameStatus   `json:"game"`
	InstallDirectory        string       `json:"installDirectory"`
	DefaultInstallDirectory string       `json:"defaultInstallDirectory"`
	SetupComplete           bool         `json:"setupComplete"`
	MusicVolume             float64      `json:"musicVolume"`
	MusicMuted              bool         `json:"musicMuted"`
	MusicAvailable          bool         `json:"musicAvailable"`
	MultiplayerConfigured   bool         `json:"multiplayerConfigured"`
	SignedIn                bool         `json:"signedIn"`
	AccountHandle           string       `json:"accountHandle"`
	AccountID               string       `json:"accountId"`
	ServerIssuer            string       `json:"serverIssuer"`
	ServerAPI               string       `json:"serverApi"`
	Realms                  []RealmState `json:"realms"`
	SelectedRealm           string       `json:"selectedRealm"`
	UpdateChannel           string       `json:"updateChannel"`
	DeveloperChannel        bool         `json:"developerChannel"`
}

type RealmState struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

func NewApp(launcher *launch.Service, tokens auth.TokenVault) *App {
	app := &App{launcher: launcher, tokens: tokens}
	app.openBrowser = func(location string) error { wailsruntime.BrowserOpenURL(app.ctx, location); return nil }
	app.devicePrompt = func(prompt auth.DevicePrompt) error {
		wailsruntime.EventsEmit(app.ctx, "auth:device-prompt", map[string]string{"verificationUri": prompt.VerificationURI, "verificationUriComplete": prompt.VerificationURIComplete, "userCode": prompt.UserCode})
		if prompt.VerificationURIComplete != "" {
			return app.openBrowser(prompt.VerificationURIComplete)
		}
		return nil
	}
	return app
}
func (a *App) ConfigureInstaller(installer *patch.Installer, manifestURL string, client *http.Client) {
	a.installer, a.manifestURL, a.downloadClient = installer, manifestURL, client
}
func (a *App) ConfigureIdentityClient(client *http.Client) { a.identityClient = client }
func (a *App) configureQuit(quit func())                   { a.quit = quit }
func (a *App) configureStateChanged(notify func())         { a.stateChanged = notify }
func (a *App) configureDeveloperInstaller(installer *patch.DeveloperInstaller) {
	a.developer = installer
	if installer != nil {
		installer.Progress = func(received, total int64) {
			progress := -1
			if total > 0 {
				progress = int(received * 100 / total)
			}
			a.mu.Lock()
			status := a.status
			status.Progress = progress
			a.status = status
			a.mu.Unlock()
		}
	}
}
func (a *App) ConfigureExperience(store *settingsStore, settings LauncherSettings, defaultRoot string, refresh auth.RefreshStore) {
	a.settingsStore, a.settings, a.defaultRoot, a.refreshStore = store, settings, defaultRoot, refresh
}
func (a *App) ConfigureMultiplayerBuild(protocol uint32, buildHash string) {
	a.protocol, a.buildHash = protocol, buildHash
}
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go func() {
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		_ = a.restoreSession(restoreCtx)
		if a.stateChanged != nil {
			a.stateChanged()
		}
	}()
}

func (a *App) PlayOffline() error {
	if a.launcher == nil {
		return errors.New("offline launcher is unavailable")
	}
	return a.finishLaunch(a.launcher.PlayOffline(a.ctx))
}

// finishLaunch is shared by every launch mode so future multiplayer hand-off
// closes the launcher only after the same supervised success contract.
func (a *App) finishLaunch(err error) error {
	if err != nil {
		return err
	}
	if a.quit != nil {
		a.quit()
	}
	return nil
}

func (a *App) GamePath() string {
	if a.launcher == nil {
		return ""
	}
	return a.launcher.GamePath()
}
func (a *App) SetGamePath(path string) error {
	if a.launcher == nil {
		return errors.New("offline launcher is unavailable")
	}
	return a.launcher.SetGamePath(path)
}

func (a *App) GetGameStatus() GameStatus {
	a.mu.RLock()
	current := a.status
	a.mu.RUnlock()
	if current.Busy {
		return current
	}
	if a.launcher == nil {
		return GameStatus{Title: "LAUNCHER UNAVAILABLE", Message: "Launcher unavailable."}
	}
	path, err := a.launcher.ResolvedGamePath()
	if err != nil || !usableExecutable(path) {
		if current.Title != "" {
			return current
		}
		return GameStatus{Title: "INSTALL REQUIRED", Message: "Choose Install or Update to prepare your first deployment.", Version: "NEW INSTALL"}
	}
	version := "INSTALLED"
	if a.installer != nil {
		if value, err := a.installer.CurrentVersion(); err == nil && value != "" {
			version = value
		}
	}
	if value := legacyVersion(a.installDirectory()); version == "INSTALLED" && value != "" {
		version = value
		if !strings.HasPrefix(strings.ToLower(version), "v") {
			version = "v" + version
		}
	}
	message := current.Message
	if message == "" {
		message = "Your installed game is ready. Offline play never requires an account or network."
	}
	return GameStatus{Installed: true, Title: "READY FOR DEPLOYMENT", Message: message, Version: version, Progress: 100}
}

func usableExecutable(path string) bool {
	info, err := os.Stat(path) // #nosec G703 -- path is the user's configured local executable.
	return err == nil && !info.IsDir()
}

func (a *App) GetLauncherState() LauncherState {
	a.mu.RLock()
	settings, signedIn, account := a.settings, a.signedIn, a.account
	realms := append([]multiplayer.Realm(nil), a.realms...)
	a.mu.RUnlock()
	_, musicErr := os.Stat(localMusicPath())
	developerChannel := a.manifestURL == "" && a.developer != nil
	channel := "SIGNED GAME MANIFEST"
	if developerChannel {
		channel = "UNSIGNED DEVELOPER BUILD"
	}
	realmStates := make([]RealmState, 0, len(realms))
	for _, realm := range realms {
		realmStates = append(realmStates, RealmState{ID: realm.ID, Name: realm.Name, Available: realm.Status == "online" && realm.AdmissionOpen})
	}
	return LauncherState{Game: a.GetGameStatus(), InstallDirectory: settings.InstallDirectory, DefaultInstallDirectory: a.defaultRoot, SetupComplete: settings.SetupComplete, MusicVolume: settings.MusicVolume, MusicMuted: settings.MusicMuted, MusicAvailable: musicErr == nil, MultiplayerConfigured: settings.Server.Issuer != "" && settings.Server.ControlAPIBaseURL != "" && settings.Server.LauncherClientID != "" && settings.Server.DeviceClientID != "", SignedIn: signedIn, AccountHandle: account.Handle(), AccountID: account.ID, ServerIssuer: settings.Server.Issuer, ServerAPI: settings.Server.ControlAPIBaseURL, Realms: realmStates, SelectedRealm: settings.SelectedRealm, UpdateChannel: channel, DeveloperChannel: developerChannel}
}

func (a *App) installDirectory() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings.InstallDirectory
}

func (a *App) InstallOrUpdate() error {
	if a.installer == nil || (a.manifestURL == "" && a.developer == nil) {
		return errors.New("game update service is unavailable; installed builds can still play offline")
	}
	installed := a.GetGameStatus().Installed
	title, message := "INSTALLING GAME", "Preparing your first deployment…"
	if installed {
		title, message = "DOWNLOADING UPDATE", "Retrieving the latest signed mission files…"
	}
	if a.manifestURL == "" {
		title, message = "INSTALLING UNSIGNED DEV BUILD", "Downloading the original developer build channel…"
		if installed {
			title = "UPDATING UNSIGNED DEV BUILD"
		}
	}
	a.setStatus(GameStatus{Installed: installed, Busy: true, Title: title, Message: message, Version: "WORKING", Progress: -1})
	var err error
	if a.manifestURL != "" {
		err = a.installer.FetchAndInstall(a.ctx, a.downloadClient, a.manifestURL)
	} else {
		_, err = a.developer.InstallOrUpdate(a.ctx)
	}
	if errors.Is(err, patch.ErrNotNewer) {
		err = nil
	}
	if err != nil {
		a.setStatus(GameStatus{Installed: installed, Title: "UPDATE NEEDS ATTENTION", Message: err.Error(), Version: "RETRY"})
		return err
	}
	a.setStatus(GameStatus{Installed: true, Title: "READY FOR DEPLOYMENT", Message: "Game installation is current.", Progress: 100})
	return nil
}

func (a *App) setStatus(status GameStatus) { a.mu.Lock(); a.status = status; a.mu.Unlock() }

func (a *App) CompleteSetup() error {
	a.mu.Lock()
	a.settings.SetupComplete = true
	settings := a.settings
	a.mu.Unlock()
	if a.settingsStore == nil {
		return nil
	}
	return a.settingsStore.Save(settings)
}

func (a *App) ChooseInstallDirectory() (LauncherState, error) {
	selected, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Choose FFReStart install folder", DefaultDirectory: nearestExistingDirectory(a.installDirectory())})
	if err != nil || selected == "" {
		return a.GetLauncherState(), err
	}
	if err := a.applyInstallDirectory(selected); err != nil {
		return a.GetLauncherState(), err
	}
	return a.GetLauncherState(), nil
}

func (a *App) ResetInstallDirectory() (LauncherState, error) {
	if err := a.applyInstallDirectory(a.defaultRoot); err != nil {
		return a.GetLauncherState(), err
	}
	return a.GetLauncherState(), nil
}

func (a *App) applyInstallDirectory(value string) error {
	root, err := validateInstallRoot(value)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.settings.InstallDirectory = root
	settings := a.settings
	if a.installer != nil {
		a.installer.Root = root
	}
	if a.developer != nil {
		a.developer.Root = root
	}
	a.mu.Unlock()
	if a.settingsStore != nil {
		if err := a.settingsStore.Save(settings); err != nil {
			return err
		}
	}
	a.setStatus(GameStatus{})
	return nil
}

func (a *App) SaveMusicPreferences(volume float64, muted bool) error {
	a.mu.Lock()
	a.settings.MusicVolume, a.settings.MusicMuted = normaliseVolume(volume), muted
	settings := a.settings
	a.mu.Unlock()
	if a.settingsStore == nil {
		return nil
	}
	return a.settingsStore.Save(settings)
}

func (a *App) OpenCommunity() error { return openExternalURL("https://discord.gg/Q5je3v9Bjg") }
func (a *App) OpenSupport() error   { return openExternalURL("https://discord.gg/VNVjmPn2Fn") }
func (a *App) OpenGameFiles() error { return openInstallFolder(a.installDirectory()) }

func (a *App) ImportDeveloperServerConfig() (LauncherState, error) {
	selected, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Import developer server configuration",
		Filters: []wailsruntime.FileFilter{{DisplayName: "Developer launcher config", Pattern: "dev-launcher-config.json"}},
	})
	if err != nil || selected == "" {
		return a.GetLauncherState(), err
	}
	config, err := loadServerConfiguration(selected)
	if err != nil {
		return a.GetLauncherState(), err
	}
	if err := a.applyServerConfiguration(config); err != nil {
		return a.GetLauncherState(), err
	}
	return a.GetLauncherState(), nil
}

func (a *App) applyServerConfiguration(config ServerConfiguration) error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	validated, err := validateServerConfiguration(config)
	if err != nil {
		return err
	}
	if a.refreshStore != nil {
		if err := a.refreshStore.Clear(); err != nil {
			return fmt.Errorf("clear the previous multiplayer sign-in: %w", err)
		}
	}
	a.mu.Lock()
	a.settings.Server = validated
	a.settings.AuthFlow = ""
	a.settings.SelectedRealm = ""
	a.accessToken, a.idToken, a.signedIn = "", "", false
	a.account, a.realms = multiplayer.Account{}, nil
	settings := a.settings
	a.mu.Unlock()
	return a.saveSettings(settings)
}

func (a *App) SignInBrowser(remember bool) error {
	config, err := a.serverConfiguration()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	if remember && a.refreshStore != nil {
		if restoreErr := a.restoreSession(ctx); restoreErr == nil {
			return nil
		}
	}
	scopes := signInScopes(config.Scopes, remember)
	tokens, err := auth.BrowserLogin(ctx, auth.BrowserConfig{Issuer: config.Issuer, ClientID: config.LauncherClientID, AuthURL: config.Issuer + "/oauth/v2/authorize", TokenURL: config.Issuer + "/oauth/v2/token", Scopes: scopes, Client: a.identityClient}, a.openBrowser)
	if err != nil {
		return err
	}
	return a.acceptTokens(ctx, tokens, "browser", remember)
}

func (a *App) SignInWithCode(remember bool) error {
	config, err := a.serverConfiguration()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	tokens, err := auth.DeviceLogin(ctx, auth.DeviceConfig{ClientID: config.DeviceClientID, DeviceURL: config.Issuer + "/oauth/v2/device_authorization", TokenURL: config.Issuer + "/oauth/v2/token", Scopes: signInScopes(config.Scopes, remember), Client: a.identityClient, PollInterval: a.devicePoll}, a.devicePrompt)
	if err != nil {
		return err
	}
	return a.acceptTokens(ctx, tokens, "device", remember)
}

func (a *App) acceptTokens(ctx context.Context, tokens auth.Tokens, flow string, remember bool) error {
	if tokens.AccessToken == "" {
		return errors.New("sign-in response omitted access token")
	}
	api, err := a.multiplayerClient()
	if err != nil {
		return err
	}
	account, err := api.Me(ctx, tokens.AccessToken)
	if errors.Is(err, multiplayer.ErrUnauthorized) && tokens.RefreshToken != "" {
		refreshed, refreshErr := a.refreshTransient(ctx, tokens.RefreshToken, flow)
		if refreshErr != nil {
			return fmt.Errorf("fresh multiplayer sign-in required: %w", refreshErr)
		}
		tokens = refreshed
		account, err = api.Me(ctx, tokens.AccessToken)
	}
	if err != nil {
		return fmt.Errorf("load multiplayer account: %w", err)
	}
	realms, err := api.Realms(ctx)
	if err != nil {
		return fmt.Errorf("load multiplayer realms: %w", err)
	}
	if remember && tokens.RefreshToken != "" && a.refreshStore != nil {
		if err := a.refreshStore.Save(tokens.RefreshToken); err != nil {
			return err
		}
	} else if a.refreshStore != nil {
		if err := a.refreshStore.Clear(); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.accessToken, a.idToken, a.signedIn = tokens.AccessToken, tokens.IDToken, true
	a.account, a.realms = account, realms
	if remember && tokens.RefreshToken != "" {
		a.settings.AuthFlow = flow
	} else {
		a.settings.AuthFlow = ""
	}
	if !realmExists(realms, a.settings.SelectedRealm) {
		a.settings.SelectedRealm = firstAvailableRealm(realms)
	}
	settings := a.settings
	a.mu.Unlock()
	return a.saveSettings(settings)
}

func (a *App) refreshTransient(ctx context.Context, refreshToken, flow string) (auth.Tokens, error) {
	config, err := a.serverConfiguration()
	if err != nil {
		return auth.Tokens{}, err
	}
	clientID := config.LauncherClientID
	if flow == "device" {
		clientID = config.DeviceClientID
	} else if flow != "browser" {
		return auth.Tokens{}, auth.ErrReauthenticationRequired
	}
	store := &auth.MemoryStore{}
	if err := store.Save(refreshToken); err != nil {
		return auth.Tokens{}, err
	}
	return auth.RefreshWithClient(ctx, config.Issuer+"/oauth/v2/token", clientID, store, a.identityClient)
}

func (a *App) SignOut() error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	a.mu.Lock()
	a.accessToken, a.idToken, a.signedIn = "", "", false
	a.account, a.realms = multiplayer.Account{}, nil
	a.settings.AuthFlow = ""
	settings := a.settings
	a.mu.Unlock()
	var clearErr error
	if a.refreshStore != nil {
		clearErr = a.refreshStore.Clear()
	}
	return errors.Join(clearErr, a.saveSettings(settings))
}

func (a *App) SelectRealm(realmID string) error {
	a.mu.Lock()
	if !realmExists(a.realms, realmID) {
		a.mu.Unlock()
		return errors.New("select a realm offered by the configured server")
	}
	a.settings.SelectedRealm = realmID
	settings := a.settings
	a.mu.Unlock()
	return a.saveSettings(settings)
}

func (a *App) PlayMultiplayer() error {
	if a.launcher == nil {
		return errors.New("multiplayer launcher is unavailable")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	accessToken, err := a.authorizedToken(ctx)
	if err != nil {
		return err
	}
	a.mu.RLock()
	realmID, realms := a.settings.SelectedRealm, append([]multiplayer.Realm(nil), a.realms...)
	a.mu.RUnlock()
	if !realmAvailable(realms, realmID) {
		return errors.New("the selected multiplayer realm is closed")
	}
	api, err := a.multiplayerClient()
	if err != nil {
		return err
	}
	protocol, buildHash := a.launchMetadata()
	ticket, err := api.LaunchTicket(ctx, accessToken, realmID, protocol, buildHash)
	if errors.Is(err, multiplayer.ErrUnauthorized) {
		if err = a.restoreSession(ctx); err != nil {
			return err
		}
		a.mu.RLock()
		accessToken = a.accessToken
		a.mu.RUnlock()
		ticket, err = api.LaunchTicket(ctx, accessToken, realmID, protocol, buildHash)
	}
	if err != nil {
		return err
	}
	ticketBytes := []byte(ticket.Value)
	ticket.Value = ""
	defer clear(ticketBytes)
	bootstrap := ticket.Bootstrap
	return a.finishLaunch(a.launcher.PlayMultiplayer(a.ctx, ticketBytes, launch.LaunchBootstrap{RealmID: bootstrap.RealmID, WorldEndpoint: bootstrap.WorldEndpoint, GNSCAKeyID: bootstrap.GNSCAKeyID, ProtocolMin: bootstrap.ProtocolMin, ProtocolMax: bootstrap.ProtocolMax, ContentVersion: bootstrap.ContentVersion}))
}

func (a *App) launchMetadata() (uint32, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.settings.Server.ProtocolVersion != 0 && a.settings.Server.BuildHash != "" {
		return a.settings.Server.ProtocolVersion, a.settings.Server.BuildHash
	}
	return a.protocol, a.buildHash
}

func (a *App) authorizedToken(ctx context.Context) (string, error) {
	a.mu.RLock()
	token := a.accessToken
	a.mu.RUnlock()
	if token != "" {
		return token, nil
	}
	if err := a.restoreSession(ctx); err != nil {
		return "", err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.accessToken, nil
}

func (a *App) restoreSession(ctx context.Context) error {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	return a.restoreSessionLocked(ctx)
}

func (a *App) restoreSessionLocked(ctx context.Context) error {
	config, err := a.serverConfiguration()
	if err != nil {
		return err
	}
	if a.refreshStore == nil {
		return auth.ErrReauthenticationRequired
	}
	a.mu.RLock()
	flow := a.settings.AuthFlow
	a.mu.RUnlock()
	clientID := config.LauncherClientID
	if flow == "device" {
		clientID = config.DeviceClientID
	} else if flow != "browser" {
		return auth.ErrReauthenticationRequired
	}
	tokens, err := auth.RefreshWithClient(ctx, config.Issuer+"/oauth/v2/token", clientID, a.refreshStore, a.identityClient)
	if err != nil {
		if errors.Is(err, auth.ErrReauthenticationRequired) {
			a.clearSession()
			return fmt.Errorf("fresh multiplayer sign-in required: %w", err)
		}
		return fmt.Errorf("refresh multiplayer sign-in: %w", err)
	}
	return a.acceptTokens(ctx, tokens, flow, true)
}

func (a *App) clearSession() {
	a.mu.Lock()
	a.accessToken, a.idToken, a.signedIn = "", "", false
	a.account, a.realms = multiplayer.Account{}, nil
	a.settings.AuthFlow = ""
	settings := a.settings
	a.mu.Unlock()
	_ = a.saveSettings(settings)
}

func (a *App) serverConfiguration() (ServerConfiguration, error) {
	a.mu.RLock()
	config := a.settings.Server
	a.mu.RUnlock()
	if config.Issuer == "" || config.ControlAPIBaseURL == "" || config.LauncherClientID == "" || config.DeviceClientID == "" {
		return ServerConfiguration{}, errors.New("multiplayer server is not configured yet")
	}
	return config, nil
}

func (a *App) multiplayerClient() (*multiplayer.Client, error) {
	config, err := a.serverConfiguration()
	if err != nil {
		return nil, err
	}
	return multiplayer.New(multiplayer.Config{BaseURL: config.ControlAPIBaseURL, BootstrapURL: config.BootstrapURL, Client: a.identityClient})
}

func (a *App) saveSettings(settings LauncherSettings) error {
	if a.settingsStore == nil {
		return nil
	}
	return a.settingsStore.Save(settings)
}

func signInScopes(scopes string, remember bool) []string {
	values := strings.Fields(scopes)
	if remember {
		return values
	}
	result := values[:0]
	for _, scope := range values {
		if scope != "offline_access" {
			result = append(result, scope)
		}
	}
	return result
}

func realmExists(realms []multiplayer.Realm, realmID string) bool {
	for _, realm := range realms {
		if realm.ID == realmID {
			return true
		}
	}
	return false
}

func realmAvailable(realms []multiplayer.Realm, realmID string) bool {
	for _, realm := range realms {
		if realm.ID == realmID {
			return realm.Status == "online" && realm.AdmissionOpen
		}
	}
	return false
}

func firstAvailableRealm(realms []multiplayer.Realm) string {
	for _, realm := range realms {
		if realm.Status == "online" && realm.AdmissionOpen {
			return realm.ID
		}
	}
	if len(realms) > 0 {
		return realms[0].ID
	}
	return ""
}
