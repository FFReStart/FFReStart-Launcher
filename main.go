package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
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
	store, err := newSettingsStore()
	if err != nil {
		log.Fatal(err)
	}
	settings := store.Load(gameRoot)
	installer := &patch.Installer{Root: settings.InstallDirectory, KeyID: gameKeyID, PublicKey: gameKey, Release: releaseMode == "true"}
	gamePath := os.Getenv("FFRESTART_GAME_PATH")
	launcher := launch.NewService(gamePath, launch.ExecStarter{}, checker)
	launcher.SetInstalledGame(installer, installedGameExecutable())
	launcher.SetUpdateTimeout(1500 * time.Millisecond)
	refresh := &auth.FallbackStore{Primary: auth.KeyringStore{Service: auth.KeyringService, User: "refresh-token"}, Memory: &auth.MemoryStore{}}
	app := NewApp(launcher, refresh)
	app.ConfigureInstaller(installer, os.Getenv("FFRESTART_GAME_MANIFEST_URL"), nil)
	app.ConfigureExperience(store, settings, gameRoot, refresh, authConfig{Issuer: os.Getenv("FFRESTART_ZITADEL_ISSUER"), ClientID: os.Getenv("FFRESTART_ZITADEL_CLIENT_ID")})
	frontendAssets, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	if err := wails.Run(&options.App{
		Title:            "FF:ReStart Launcher",
		Width:            1100,
		Height:           680,
		MinWidth:         880,
		MinHeight:        620,
		AssetServer:      &assetserver.Options{Assets: frontendAssets, Handler: launcherAssetHandler()},
		BackgroundColour: &options.RGBA{R: 6, G: 17, B: 29, A: 1},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
	}); err != nil {
		log.Print(err)
	}
}

func localMusicPath() string {
	root, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(root, "FFReStart", "launcher", "audio", "launcher-main-theme.mp3")
}

func launcherAssetHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if request.URL.Path == "/launcher/audio/launcher-main-theme.mp3" {
			path := localMusicPath()
			if path == "" {
				http.NotFound(writer, request)
				return
			}
			writer.Header().Set("Cache-Control", "no-store")
			http.ServeFile(writer, request, path)
			return
		}
		http.NotFound(writer, request)
	})
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
