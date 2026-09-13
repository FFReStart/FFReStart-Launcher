# Wails packaging

Wails platform metadata, icons, installers, and signing inputs live here. Real
signing material is never committed; release workflows obtain it only from the
protected `release` environment.

`appicon.png` and `windows/icon.ico` are derived from the already-public
`GameLauncher/images/ReStartTransparentLogo.png` on `origin/main`. The Windows
ICO contains 16, 24, 32, 48, 64, 128 and 256 pixel variants; `appicon.png` is
also embedded as the Linux window/taskbar icon.
