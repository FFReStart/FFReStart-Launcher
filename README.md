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

## Login and pre-authenticated launch

Create a local account through the game once, then sign in from the launcher. Play starts the game with the shared `Game.exe --auth-token <token>` contract; the password is never sent to the game.

“Remember Password” is opt-in and stores a revalidated derived session—not the password—in a Windows current-user DPAPI-protected blob. See [Launcher authentication](docs/authentication.md) for the token format, storage location, cleanup behavior, threat model, and integration details.
