# Wails launcher proof of concept

## Recommendation: GO, with an updater caveat

Wails can do every launcher-critical job tested here, and Go is a materially better fit for this team than security-critical custom Rust. Adopt Wails v2 for the launcher, conditional on treating the custom updater as a separately reviewed security component. Wails v2 has no official updater comparable to Tauri's plugin, so that work does not disappear.

## Pinned toolchain and dependencies

- Go 1.27.1; Wails CLI/module **v2.15.0**; Node 22.12.0; pnpm 9.15.1.
- TypeScript 5.6.3 and Vite 7.0.0 are exact in `package.json`/`pnpm-lock.yaml`.
- `x/oauth2` v0.37.0, `go-oidc/v3` v3.21.0, `go-keyring` v0.2.8, `go-selfupdate` v1.6.0, protobuf-go v1.36.10, and Buf image 1.57.0.
- Choose v2: the module proxy exposes v2.15.0 as a release, while the newest v3 observed on 2026-09-12 is **v3.0.0-beta.20**, explicitly prerelease software. Wails says v2 remains stable and maintained while v3 is beta ([releases](https://github.com/wailsapp/wails/releases), [v3 status](https://v3.wails.io/blog/tags/wails/)). Re-evaluate after v3 GA, not before.

## Results against the eight gates

1. **Version — pass.** The module and installed CLI are pinned to v2.15.0; no `latest` or local module replacement is committed.
2. **Window/build — pass.** The TypeScript window has **Sign in**, **Sign in with a code**, and **Play**, a restrictive CSP, and no token-bearing UI API. `wails build -platform windows/amd64 -clean` produced one `wails-spike.exe`, 11,791,872 bytes (11.25 MiB). Window-ready times on this Windows 11 machine were 641/657/793/757/685 ms (median 685 ms); the first cold measurement was 1,455 ms. WebView2 is supplied by Windows 11, not bundled.
3. **Browser login — pass.** `TestBrowserLoginUsesLoopbackS256StateAndNonce` uses `golang.org/x/oauth2`, binds `127.0.0.1:0`, proves a random port, requires `code_challenge_method=S256`, checks callback state, sends the verifier, verifies the RS256 ID token through discovery/JWKS, and checks nonce. The provider is an in-process mock; no shared ZITADEL stack was touched.
4. **Device code — pass.** `TestDeviceCodePolling` proves the RFC 8628 grant type, user/complete verification URIs, `authorization_pending`, polling, and successful token receipt against an in-process provider.
5. **Refresh storage — pass.** `go-keyring` wrote/read/deleted a uniquely named `FFReStart-wails-spike-test` Windows Credential Manager entry. Failure falls back to process memory only. `TestInvalidGrantClearsRefreshToken` proves `invalid_grant` clears it and returns `ErrReauthenticationRequired`.
6. **Game hand-off — pass.** The copied contract was compiled to Go by protoc-gen-go through pinned Buf. `TestLaunchHandoffIsLengthDelimitedOnStdin` builds a dummy game, starts it with only `--auth-token-stdin`, writes exactly one length-delimited `LaunchHandoff`, closes stdin, and gets decoded realm `academy`; the ticket is absent from argv/output.
7. **Signed update — pass at mechanism level.** A local HTTP origin serves an Ed25519-signed manifest; the compiled-in public key verifies it, semantic versions reject equal/downgrade, and downloaded bytes must match SHA-256. Only those verified bytes reach the actively released [creativeprojects/go-selfupdate](https://github.com/creativeprojects/go-selfupdate) v1.6.0 apply engine. Tests reject a modified manifest and modified binary. On Windows, `TestWindowsReplacesExecutableWhileItIsRunning` starts a real `.exe` and proves replacement of its on-disk image while it is still running, using rollback-safe rename behavior.
8. **Linux/macOS — Linux build passed; macOS documented, not run.** A Debian 12 Docker build produced a 5,808,652-byte linux/amd64 ELF using `libgtk-3-dev`, `libwebkit2gtk-4.1-dev`, `pkg-config`, `build-essential`, and exact tag `webkit2_41`. Ubuntu 24.04 uses the same packages/tag; without that tag Wails v2 asks for obsolete WebKitGTK 4.0. Ubuntu 22.04/Debian 12/SteamOS packaging and runtime library installation still need a matrix. macOS needs Xcode command-line tools; build universal on macOS (`wails build -platform darwin/universal`), sign the `.app` with Developer ID + hardened runtime (`codesign --options runtime`), submit with `xcrun notarytool`, then `xcrun stapler staple`. Intel/Rosetta and Apple-silicon tests are still required. See Wails [installation prerequisites](https://wails.io/docs/gettingstarted/installation/).

## Wails versus Tauri on the points that matter

| Point | Wails v2 | Tauri 2 |
|---|---|---|
| Updater | Custom Go manifest/download/apply/restart and installer policy; mechanism proven here, but ours to secure | Official signed updater plugin; lower launcher-update effort |
| OS support | Native WebView2/WKWebView/WebKitGTK; meets the chosen Windows 10 22H2, macOS 14, Ubuntu 22.04/Debian 12 floor; 4.1 needs `webkit2_41` | Also meets the floor; WebKitGTK 4.1 path is more standardised |
| Team fit | Go backend + TypeScript UI; matches existing Go/TS ownership and removes Rust | TypeScript UI, but ~1.5–2k estimated lines of unfamiliar Rust in auth/patch/update code |

## Risks and full-launcher work

- The spike proves primitives, not production UX or ZITADEL interoperability. Wire the UI to backend-only services, use the system browser, add cancellation/timeouts, redact logs, and perform family-wide sign-out after refresh reuse.
- Bind only narrow Wails methods; never expose tokens to JavaScript. Keep CSP strict and render server text as text. Add single-instance behavior and secure log/settings handling.
- Harden updater parsing/limits, key rotation/revocation and anti-freeze policy; add restart/rollback recovery, Authenticode/Developer ID/Linux packaging, protected offline signing, and 1,000-mutation tests. The game chunk patcher remains separate substantial work.
- Test Windows 10 22H2, macOS 14 universal, Ubuntu 22.04/24.04, Debian 12 and SteamOS Gaming Mode. Secret Service absence must visibly mean memory-only login.
- Wails v2's maintenance horizon is a risk as v3 approaches GA. Budget a migration spike only after a stable v3 release and a settled Linux webview story.

## Reproduction

From `wails-spike`: `just test`, `just build-windows`, and `just generate-proto`. All automated tests pass on Windows; generated/built artifacts are ignored. Existing .NET projects were not modified.
