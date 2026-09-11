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

The launcher installs and updates the game under `%LOCALAPPDATA%\FFReStart`. This directory also contains `Version.txt` and any temporary game download.

## Launcher UI conventions

The WPF UI uses a small resource-driven visual system in `GameLauncher/App.xaml`: deep navy surfaces, cyan information accents, and green primary/action states. Keep the Play/Install/Update action dominant and reserve the secondary action row for real destinations and utilities.

`AccountStatusHost` in `MainWindow.xaml` is the merge point for account/auth work. Auth changes can replace its content while retaining the header layout; the default guest state is intentionally informative rather than interactive.

Community and Support are separate destinations exposed through `LauncherLinks.GetUrl`. Both open through Windows' safe default-browser shell behavior. Run the focused contract checks with:

```powershell
dotnet run --project GameLauncher.ContractTests/GameLauncher.ContractTests.csproj
```

For UI-only local review without contacting the update service or launching the game:

```powershell
dotnet run --project "GameLauncher/FFReStart GameLauncher.csproj" -- --preview
```
