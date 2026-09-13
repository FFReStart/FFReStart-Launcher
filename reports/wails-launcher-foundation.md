# Wails launcher foundation

## Result

This branch brings the successful `origin/wails-spike` approach into the
production repository layout selected in D14. It pins Wails v2.15.0, Go 1.27.1,
Node 22.12.0 and pnpm 9.15.1; adds `frontend/`,
`internal/{auth,update,patch,launch}/`, `contracts/{proto/launch/v1,fixtures}/`
and `build/`; and leaves every existing .NET launcher file in place.

The JavaScript boundary binds only game-path and offline-play methods. No
credential type or token-bearing operation is available to the frontend.

## Foundation delivered

- A production Wails application and TypeScript/Vite offline-first UI with a
  restrictive content security policy.
- One `just check` gate for frontend install, typecheck and build, Go vet and
  tests, and golangci-lint (`errcheck`, `exhaustive`, `gosec`, `staticcheck`).
- Pull-request CI with immutable action SHAs, checks, a Windows Wails build, and
  a Dockerized Linux build using WebKitGTK 4.1 and `webkit2_41`.
- Windows and Linux build recipes plus a release build wrapper that requires an
  injected Ed25519 public key and rejects test key IDs. Private signing keys are
  neither accepted nor present in the repository.
- The launch v1 Protobuf source and fixture location needed for later WP8
  hand-off compatibility work.

Dependency record:

| Dependency | Purpose | Licence / maintenance | Alternative and removal path |
|---|---|---|---|
| Wails v2.15.0 | Native window, bindings and packaging | MIT; stable v2 release used by the accepted spike | Tauri 2 remains the architecture runner-up; replace the app shell and bindings if the WP26 matrix fails |
| go-selfupdate v1.6.0 | Apply already verified launcher bytes with Windows-safe replacement | MIT; active release validated by the spike | Replace behind `HTTPChecker.FetchAndApply` without changing manifest verification |
| x/mod v0.37.0 | Strict semantic-version comparisons | BSD-3-Clause; Go project module | Replace with a small reviewed SemVer library if the Go module is removed |
| TypeScript 5.6.3 / Vite 7.0.0 | Typed frontend and static bundling | Apache-2.0 / MIT; exact lockfile pins | Replace the frontend builder without widening the Wails API |

## WP32: offline launcher path

Met in this session:

- `PlayOffline` starts the configured executable with only `--offline`; it does
  not sign in, request a ticket, contact the control API, or import `internal/auth`.
- The Wails app has no token-bearing method. `TestOfflineAppHasNoTokenPath`
  uses a panic-on-access token sentinel and proves zero reads and writes.
- The game start completes before a blocking update checker can return.
  `TestOfflineLaunchDoesNotWaitForUpdate` enforces a 100 ms upper bound while
  the checker remains blocked.
- A process-lifetime `sync.Once` permits at most one update check. It runs with
  a 1.5 second context deadline and its failure is ignored by the play path.
- `TestNetworkUnavailableAllowsTwentyOfTwentyLaunches` proves 20/20 simulated
  starts when the update service reports the network unavailable.
- The game path is configurable through `FFRESTART_GAME_PATH` or the narrow UI
  setter. Persistent settings can be added alongside installation in a later
  package.

Still open:

- Run 20/20 real cold launches on Windows, Linux and macOS with DNS and network
  disabled, and retain packet captures proving that the optional update probe
  is the only network request.
- Measure real launcher-to-game times with the update service available and
  unavailable and prove the difference is within 5%.
- Persist and validate the installed game path once installer ownership and
  patch installation paths are defined.

## WP9: updater basics

Met in this session:

- A bounded, strict JSON manifest parser and fuzz target.
- Ed25519 verification over canonical manifest fields with a pinned key ID.
- Semantic-version rejection of equal releases and downgrades.
- A bounded download whose SHA-256 must match before bytes reach pinned
  go-selfupdate v1.6.0.
- Tests for a valid apply, tampered manifests, tampered binaries, equal versions,
  downgrades, missing release keys, and test-key refusal.
- The production public key is empty in source and supplied to release builds
  as linker values. The only private key is deterministically constructed in a
  `_test.go` file and is explicitly named test-only. No real signing material is
  present.

Still open:

- Resumable launcher and game downloads, content-addressed game chunks, atomic
  promotion, and repair behavior.
- Retaining a known-good launcher, restart/interruption recovery, and automatic
  rollback. The apply engine provides the verified replacement primitive, not
  the complete recovery policy.
- Key rotation, revocation and anti-freeze policy plus their mutation tests.
- The protected `release` environment signing/promote workflow, fork and
  non-release denial tests, human approval, and offline backup drill from G1.
- Authenticode, Developer ID, Linux packaging and platform installer policy.

Resumable downloads and rollback were intentionally left for the remaining WP9
sessions rather than added without the staging and recovery design they need.

## Follow-on packages

WP8 must add optional multiplayer without changing the offline default: RFC
8252 loopback PKCE and RFC 8628 device login in Go, memory-only access/ID tokens,
remember-me refresh tokens in the OS keyring with memory fallback, invalid-grant
cleanup and family-wide sign-out, a control-API ticket request, and one
length-delimited `LaunchHandoff` written to game stdin under
`--auth-token-stdin`. Golden fixtures must decode in both Go and Unity C#.
Tokens must remain absent from JavaScript, argv, logs and stdout.

WP26 must exercise Windows 10 22H2 and Windows 11, Ubuntu 22.04/24.04, Debian
12, SteamOS Gaming Mode, and macOS 14 on Apple silicon and Intel/Rosetta. It must
cover offline launch with no network/account and multiplayer login/update/play.
macOS needs a universal app, Developer ID signing with hardened runtime,
notarization, stapling, and validation on a dedicated Mac runner.

## .NET cutover option

Keep the .NET launcher buildable while the Wails branch completes WP8, WP9 and
WP26. Publish Wails prereleases under a distinct artifact name and collect OS
matrix evidence. After those gates pass, freeze feature work in .NET, preserve
one rollback release, switch release assets and documentation to Wails in a
separate reviewed pull request, and remove the .NET project only after one
successful release cycle. The cutover should not rewrite history or merge this
branch directly into a protected branch.

## Verification evidence

- `just check`: passed locally on Windows 11.
- `just build-windows`: produced `build/bin/ffrestart-launcher.exe` with Wails
  v2.15.0.
- `just build-linux`: produced `build/linux-out/ffrestart-launcher` in Docker
  from Go 1.27.1 with the exact `webkit2_41` tag.
- A five-second manifest fuzz run completed 461,560 executions without a
  failure, and the 20/20 unavailable-network simulation passed 20 repeated test
  runs (400 simulated game starts total).
- Secret scan of the diff: no token, secret, private key literal, PEM key, RFC
  8032 spike key, or signing material found. Test code derives its test-only key
  from a non-secret deterministic byte sequence at runtime.
