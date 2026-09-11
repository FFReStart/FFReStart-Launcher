# FFReStart-Launcher
The Launcher Repository for FFReStart.

Built with .NET 8. Tagged releases are self-contained, so users do not need to install the .NET runtime separately.

## Creating a release

Push a semantic-version tag in either `1.2.3` or `v1.2.3` format:

```powershell
git tag v1.2.3
git push origin v1.2.3
```

GitHub Actions will build a self-contained, single-file Windows x64 executable and attach it to a GitHub Release for that tag. The executable includes the launcher, its dependencies, application resources, and .NET runtime. Prerelease suffixes such as `v1.2.3-beta.1` are also supported.

## Game data location

The default game install folder is `%LOCALAPPDATA%\FFReStart`. Players can use **Change** beside **Install Folder** to choose another writable folder. The selected folder owns the downloaded archive, extracted build, and `Version.txt`; update checks, repair/install, launch discovery, offline launch, and **Game Files** all follow that selection.

Changing the folder does not move or delete an existing installation. The launcher checks the selected destination and installs there if needed, leaving the previous folder untouched so the player can remove it after verifying the new install. Launcher settings and remembered-session data remain under `%LOCALAPPDATA%\FFReStart\Launcher` regardless of the game location.

Settings from the earlier executable-picker implementation are migrated by inferring the install root from the saved game executable. Missing or invalid legacy paths safely fall back to the LocalAppData default.

## Launcher music

The header keeps launcher music controls visible without competing with the primary Play action. Players can adjust volume with the slider (including arrow-key input) or use the separate Mute toggle. Both preferences persist in the protected launcher settings under `%LOCALAPPDATA%\FFReStart\Launcher`.

The looping launcher track is the original `MenuScreen` theme from the FFReStart game project: `Legacy -Main Theme- by Panman14.ogg` (Unity asset GUID `5f1672567e1e4fd499d7b79cc004b429`). The checked-in MP3 is a Windows-compatible transcode of that source and is embedded into the launcher executable; maintainers should update it only from the project-owned source asset.

## Launcher UI conventions

The WPF UI uses a small resource-driven visual system in `GameLauncher/App.xaml`: deep navy surfaces, cyan information accents, and green primary/action states. Keep the Play/Install/Update action dominant and reserve the secondary action row for real destinations and utilities.

`AccountStatusHost` in `MainWindow.xaml` is the merge point for account/auth work. Auth changes can replace its content while retaining the header layout; the default guest state is intentionally informative rather than interactive.

Community and Support are separate destinations exposed through `LauncherLinks.GetUrl`. Both open through Windows' safe default-browser shell behavior. Run the focused contract checks with:

- Community/Discord: <https://discord.gg/Q5je3v9Bjg>
- Support: <https://discord.gg/VNVjmPn2Fn>

```powershell
dotnet run --project GameLauncher.ContractTests/GameLauncher.ContractTests.csproj
```

For UI-only local review without contacting the update service or launching the game:

```powershell
dotnet run --project "GameLauncher/FFReStart GameLauncher.csproj" -- --preview
```

## Login and pre-authenticated launch

Create a local account through the game once, then sign in from the launcher. Play starts the game with the shared `Game.exe --auth-token <token>` contract. The launcher never sends the password to the game.

“Remember Password” is opt-in and stores a revalidated derived session—not the password—in a Windows current-user DPAPI-protected blob. The launcher does not fall back to plaintext storage. See [Launcher authentication](docs/authentication.md) for setup, recovery, token format, cleanup behavior, and the actual local-account threat model.
