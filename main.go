package main

import (
	"embed"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
	"github.com/FFReStart/FFReStart-Launcher/internal/update"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

var (
	releaseMode        = "false"
	updateKeyID        = ""
	updatePublicKeyHex = ""
	gameKeyID          = ""
	gamePublicKeyHex   = ""
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	key, err := update.ReleasePublicKey(updateKeyID, updatePublicKeyHex, releaseMode == "true")
	if err != nil {
		log.Fatal(err)
	}

	var checker launch.UpdateChecker
	if manifestURL := os.Getenv("FFRESTART_UPDATE_MANIFEST_URL"); manifestURL != "" && key != nil {
		httpChecker := update.NewHTTPChecker(manifestURL, "v0.1.0", updateKeyID, key, 1<<20)
		if releaseMode == "true" {
			httpChecker.RequireHTTPS()
		}
		checker = httpChecker
	}
	gameKey, err := update.ReleasePublicKey(gameKeyID, gamePublicKeyHex, releaseMode == "true")
	if err != nil {
		log.Fatal(err)
	}
	gameRoot, err := patch.DefaultGameRoot()
	if err != nil {
		log.Fatal(err)
	}
	installer := &patch.Installer{Root: gameRoot, KeyID: gameKeyID, PublicKey: gameKey, Release: releaseMode == "true"}
	gamePath := os.Getenv("FFRESTART_GAME_PATH")
	launcher := launch.NewService(gamePath, launch.ExecStarter{}, checker)
	launcher.SetInstalledGame(installer, installedGameExecutable())
	launcher.SetUpdateTimeout(1500 * time.Millisecond)
	app := NewApp(launcher, nil)
	app.ConfigureInstaller(installer, os.Getenv("FFRESTART_GAME_MANIFEST_URL"), nil)

	if err := wails.Run(&options.App{
		Title:            "FF:ReStart Launcher",
		Width:            1100,
		Height:           680,
		MinWidth:         720,
		MinHeight:        520,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 7, G: 20, B: 38, A: 1},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
	}); err != nil {
		log.Print(err)
	}
}

func installedGameExecutable() string {
	if configured := os.Getenv("FFRESTART_GAME_EXECUTABLE"); configured != "" {
		return configured
	}
	if runtime.GOOS == "windows" {
		return "FFReStart.exe"
	}
	return "FFReStart.x86_64"
}
