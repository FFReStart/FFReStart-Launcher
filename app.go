package main

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
)

// App is the deliberately narrow JavaScript-to-Go boundary. The unexported
// vault is reserved for WP8; no token-bearing method is exposed to JavaScript.
type App struct {
	ctx         context.Context
	launcher    *launch.Service
	tokens      auth.TokenVault
	installer   *patch.Installer
	manifestURL string
	client      *http.Client
	statusMu    sync.RWMutex
	status      string
}

type GameStatus struct {
	Installed bool   `json:"installed"`
	Message   string `json:"message"`
}

func NewApp(launcher *launch.Service, tokens auth.TokenVault) *App {
	return &App{launcher: launcher, tokens: tokens}
}

func (a *App) ConfigureInstaller(installer *patch.Installer, manifestURL string, client *http.Client) {
	a.installer, a.manifestURL, a.client = installer, manifestURL, client
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

func (a *App) GetGameStatus() GameStatus {
	if a.launcher == nil {
		return GameStatus{Message: "Launcher unavailable."}
	}
	if _, err := a.launcher.ResolvedGamePath(); err != nil {
		return GameStatus{Message: "Game not installed. Connect to install it."}
	}
	a.statusMu.RLock()
	message := a.status
	a.statusMu.RUnlock()
	if message == "" {
		message = "Installed game is ready for offline play."
	}
	return GameStatus{Installed: true, Message: message}
}

func (a *App) InstallOrUpdate() error {
	if a.installer == nil || a.manifestURL == "" {
		return errors.New("game update service is unavailable")
	}
	if err := a.installer.FetchAndInstall(a.ctx, a.client, a.manifestURL); err != nil {
		return err
	}
	a.statusMu.Lock()
	a.status = "Game installation is current."
	a.statusMu.Unlock()
	return nil
}

func (a *App) SetGamePath(path string) error {
	if a.launcher == nil {
		return errors.New("offline launcher is unavailable")
	}
	return a.launcher.SetGamePath(path)
}
