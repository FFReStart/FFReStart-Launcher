# FF:ReStart Launcher

The production launcher foundation uses Wails v2.15.0 with a Go security
boundary and a TypeScript UI. The existing .NET launcher remains in
`GameLauncher/` until a separate cutover decision.

## Development

The pinned toolchain is Go 1.27.1, Node 22.12.0, pnpm 9.15.1, Wails v2.15.0,
and golangci-lint v2.13.2. Run the full local gate with:

```shell
just check
```

Build Windows locally with `just build-windows`. The Linux build is isolated in
Docker and uses WebKitGTK 4.1 through the required `webkit2_41` build tag:

```shell
just build-linux
```

Set `FFRESTART_GAME_PATH` to the game executable and optionally set
`FFRESTART_UPDATE_MANIFEST_URL` for the single best-effort background update
check. Offline play never requires either the network or an account.

Release builds inject only the Ed25519 public verification key; signing keys
belong exclusively to the protected release environment:

```shell
FFRESTART_UPDATE_KEY_ID=release-2026-01 \
FFRESTART_UPDATE_PUBLIC_KEY_HEX=<public-key> \
just build-release windows/amd64
```

The release command rejects absent keys and key IDs beginning with `test`.
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
# Local launcher music

The theme track is private and is never embedded in or committed to this public repository. Developers who have the owner's local source branch can run:

```powershell
.\scripts\install-local-music.ps1
```

The script reads the track with `git show` without checking out or changing that branch, then installs it at `%LOCALAPPDATA%\FFReStart\launcher\audio\launcher-main-theme.mp3`. The Wails launcher autoplays and loops it when available, and silently continues without music otherwise.

Optional multiplayer sign-in is enabled only when `FFRESTART_ZITADEL_ISSUER` and `FFRESTART_ZITADEL_CLIENT_ID` identify a configured public/native client. Passwords are never accepted by the launcher. Offline play is always available independently of sign-in.
