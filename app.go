package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type authConfig struct{ Issuer, ClientID string }

// App is the narrow JavaScript-to-Go boundary. Token values never appear in a
// bound argument, return value, event, log message, environment or argv.
type App struct {
	ctx           context.Context
	launcher      *launch.Service
	tokens        auth.TokenVault
	refreshStore  auth.RefreshStore
	installer     *patch.Installer
	developer     *patch.DeveloperInstaller
	manifestURL   string
	client        *http.Client
	settingsStore *settingsStore
	settings      LauncherSettings
	defaultRoot   string
	auth          authConfig

	mu                   sync.RWMutex
	status               GameStatus
	signedIn             bool
	accessToken, idToken string
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
	Game                    GameStatus `json:"game"`
	InstallDirectory        string     `json:"installDirectory"`
	DefaultInstallDirectory string     `json:"defaultInstallDirectory"`
	SetupComplete           bool       `json:"setupComplete"`
	MusicVolume             float64    `json:"musicVolume"`
	MusicMuted              bool       `json:"musicMuted"`
	MusicAvailable          bool       `json:"musicAvailable"`
	MultiplayerConfigured   bool       `json:"multiplayerConfigured"`
	SignedIn                bool       `json:"signedIn"`
	UpdateChannel           string     `json:"updateChannel"`
	DeveloperChannel        bool       `json:"developerChannel"`
}

func NewApp(launcher *launch.Service, tokens auth.TokenVault) *App {
	return &App{launcher: launcher, tokens: tokens}
}
func (a *App) ConfigureInstaller(installer *patch.Installer, manifestURL string, client *http.Client) {
	a.installer, a.manifestURL, a.client = installer, manifestURL, client
}
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
func (a *App) ConfigureExperience(store *settingsStore, settings LauncherSettings, defaultRoot string, refresh auth.RefreshStore, config authConfig) {
	a.settingsStore, a.settings, a.defaultRoot, a.refreshStore, a.auth = store, settings, defaultRoot, refresh, config
}
func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) PlayOffline() error {
	if a.launcher == nil {
		return errors.New("offline launcher is unavailable")
	}
	return a.launcher.PlayOffline(a.ctx)
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
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (a *App) GetLauncherState() LauncherState {
	a.mu.RLock()
	settings, signedIn := a.settings, a.signedIn
	a.mu.RUnlock()
	_, musicErr := os.Stat(localMusicPath())
	developerChannel := a.manifestURL == "" && a.developer != nil
	channel := "SIGNED GAME MANIFEST"
	if developerChannel {
		channel = "UNSIGNED DEVELOPER BUILD"
	}
	return LauncherState{Game: a.GetGameStatus(), InstallDirectory: settings.InstallDirectory, DefaultInstallDirectory: a.defaultRoot, SetupComplete: settings.SetupComplete, MusicVolume: settings.MusicVolume, MusicMuted: settings.MusicMuted, MusicAvailable: musicErr == nil, MultiplayerConfigured: a.auth.Issuer != "" && a.auth.ClientID != "", SignedIn: signedIn, UpdateChannel: channel, DeveloperChannel: developerChannel}
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
		err = a.installer.FetchAndInstall(a.ctx, a.client, a.manifestURL)
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

func (a *App) SignInBrowser() error {
	if a.auth.Issuer == "" || a.auth.ClientID == "" {
		return errors.New("multiplayer sign-in is not configured yet")
	}
	issuer := strings.TrimRight(a.auth.Issuer, "/")
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Minute)
	defer cancel()
	if a.refreshStore != nil {
		if refreshed, refreshErr := auth.Refresh(ctx, issuer+"/oauth/v2/token", a.auth.ClientID, a.refreshStore); refreshErr == nil {
			return a.acceptTokens(refreshed)
		}
	}
	tokens, err := auth.BrowserLogin(ctx, auth.BrowserConfig{Issuer: issuer, ClientID: a.auth.ClientID, AuthURL: issuer + "/oauth/v2/authorize", TokenURL: issuer + "/oauth/v2/token"}, func(url string) error { wailsruntime.BrowserOpenURL(a.ctx, url); return nil })
	if err != nil {
		return err
	}
	return a.acceptTokens(tokens)
}

func (a *App) SignInWithCode() error {
	if a.auth.Issuer == "" || a.auth.ClientID == "" {
		return errors.New("multiplayer sign-in is not configured yet")
	}
	issuer := strings.TrimRight(a.auth.Issuer, "/")
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	tokens, err := auth.DeviceLogin(ctx, auth.DeviceConfig{ClientID: a.auth.ClientID, DeviceURL: issuer + "/oauth/v2/device_authorization", TokenURL: issuer + "/oauth/v2/token"}, func(prompt auth.DevicePrompt) error {
		wailsruntime.EventsEmit(a.ctx, "auth:device-prompt", map[string]string{"verificationUri": prompt.VerificationURI, "verificationUriComplete": prompt.VerificationURIComplete, "userCode": prompt.UserCode})
		if prompt.VerificationURIComplete != "" {
			wailsruntime.BrowserOpenURL(a.ctx, prompt.VerificationURIComplete)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return a.acceptTokens(tokens)
}

func (a *App) acceptTokens(tokens auth.Tokens) error {
	if tokens.RefreshToken != "" && a.refreshStore != nil {
		if err := a.refreshStore.Save(tokens.RefreshToken); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.accessToken, a.idToken, a.signedIn = tokens.AccessToken, tokens.IDToken, true
	a.mu.Unlock()
	return nil
}

func (a *App) SignOut() error {
	a.mu.Lock()
	a.accessToken, a.idToken, a.signedIn = "", "", false
	a.mu.Unlock()
	if a.refreshStore != nil {
		return a.refreshStore.Clear()
	}
	return nil
}
