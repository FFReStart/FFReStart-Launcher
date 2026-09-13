package main

import (
	"embed"
	"encoding/hex"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/FFReStart/FFReStart-Launcher/internal/auth"
	"github.com/FFReStart/FFReStart-Launcher/internal/launch"
	"github.com/FFReStart/FFReStart-Launcher/internal/patch"
	"github.com/FFReStart/FFReStart-Launcher/internal/update"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	linuxoptions "github.com/wailsapp/wails/v2/pkg/options/linux"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	releaseMode                 = "false"
	updateKeyID                 = ""
	updatePublicKeyHex          = ""
	gameKeyID                   = ""
	gamePublicKeyHex            = ""
	productionIssuer            = ""
	productionControlAPI        = ""
	productionLauncherClientID  = ""
	productionDeviceClientID    = ""
	productionAudienceProjectID = ""
	productionProtocolVersion   = "1"
	productionBuildHash         = "0000000000000000000000000000000000000000000000000000000000000000"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

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
	protocol, err := strconv.ParseUint(productionProtocolVersion, 10, 32)
	buildHashBytes, hashErr := hex.DecodeString(productionBuildHash)
	if err != nil || protocol == 0 || hashErr != nil || len(buildHashBytes) != 32 {
		log.Fatal("multiplayer build metadata is invalid")
	}
	settings := store.Load(gameRoot)
	if settings.Server.Issuer == "" {
		controlAPI := firstConfigured(productionControlAPI, os.Getenv("FFRESTART_CONTROL_API_BASE_URL"))
		candidate := ServerConfiguration{
			Issuer:            firstConfigured(productionIssuer, os.Getenv("FFRESTART_ZITADEL_ISSUER")),
			ControlAPIBaseURL: controlAPI,
			BootstrapURL:      controlAPI + "/v1/bootstrap",
			LaunchJWKSURL:     controlAPI + "/.well-known/jwks.json",
			ProtocolVersion:   uint32(protocol),
			BuildHash:         strings.ToLower(productionBuildHash),
			LauncherClientID:  firstConfigured(productionLauncherClientID, os.Getenv("FFRESTART_ZITADEL_CLIENT_ID")),
			DeviceClientID:    firstConfigured(productionDeviceClientID, os.Getenv("FFRESTART_ZITADEL_DEVICE_CLIENT_ID")),
			AudienceProjectID: firstConfigured(productionAudienceProjectID, os.Getenv("FFRESTART_ZITADEL_AUDIENCE_PROJECT_ID")),
		}
		if candidate.Issuer != "" || candidate.ControlAPIBaseURL != "" || candidate.LauncherClientID != "" || candidate.DeviceClientID != "" {
			settings.Server, err = validateServerConfiguration(candidate)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	installer := &patch.Installer{Root: settings.InstallDirectory, KeyID: gameKeyID, PublicKey: gameKey, Release: releaseMode == "true"}
	gamePath := os.Getenv("FFRESTART_GAME_PATH")
	launcher := launch.NewService(gamePath, launch.ExecStarter{}, checker)
	launcher.SetInstalledGame(&gameInstallation{installer: installer}, installedGameExecutable())
	launcher.SetUpdateTimeout(1500 * time.Millisecond)
	refresh := &auth.FallbackStore{Primary: auth.KeyringStore{Service: auth.KeyringService, User: "refresh-token"}, Memory: &auth.MemoryStore{}}
	app := NewApp(launcher, refresh)
	app.configureQuit(func() { wailsruntime.Quit(app.ctx) })
	app.configureStateChanged(func() { wailsruntime.EventsEmit(app.ctx, "auth:state-changed") })
	app.ConfigureInstaller(installer, os.Getenv("FFRESTART_GAME_MANIFEST_URL"), nil)
	if os.Getenv("FFRESTART_GAME_MANIFEST_URL") == "" {
		app.configureDeveloperInstaller(&patch.DeveloperInstaller{Root: settings.InstallDirectory})
	}
	app.ConfigureExperience(store, settings, gameRoot, refresh)
	app.ConfigureMultiplayerBuild(uint32(protocol), strings.ToLower(productionBuildHash))
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
		Linux:            &linuxoptions.Options{Icon: appIcon, ProgramName: "ffrestart-launcher"},
		BackgroundColour: &options.RGBA{R: 6, G: 17, B: 29, A: 1},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
	}); err != nil {
		log.Print(err)
	}
}

func firstConfigured(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
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
		return legacyWindowsGame
	}
	return "FFReStart.x86_64"
}
