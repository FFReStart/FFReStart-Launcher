package main

import (
	"context"
	"errors"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
)

// App is the deliberately narrow JavaScript-to-Go boundary. The unexported
// vault is reserved for WP8; no token-bearing method is exposed to JavaScript.
type App struct {
	ctx      context.Context
	launcher *launch.Service
	tokens   auth.TokenVault
}

func NewApp(launcher *launch.Service, tokens auth.TokenVault) *App {
	return &App{launcher: launcher, tokens: tokens}
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
