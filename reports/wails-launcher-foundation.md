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
  downgrades, missing release keys, and test-key refusal by both ID and decoded
  public-key bytes even when a known test key is given a production-looking ID.
- The production public key is empty in source and supplied to release builds
  as linker values. The only private key is deterministically constructed in a
  `_test.go` file and is explicitly named test-only. No real signing material is
  present. The release denylist contains only public bytes for that key and the
  RFC 8032 key used by the Wails spike.

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

## WP9 session 2: resumable updates and rollback

Delivered in this session:

- Launcher and game artifacts download to `.part` files with an atomically
  persisted progress record. Retries use HTTP Range only when the partial file,
  URL, signed size and signed SHA-256 still agree. A server that ignores Range
  triggers a clean restart, while invalid ranges, truncation and oversized data
  are rejected. A completed file is promoted only after its SHA-256 matches.
- The launcher manifest now signs the artifact size as well as version, URL,
  hash and key ID. The launcher retains `.previous`, restores it after an apply
  error or failed bounded health check, and leaves only verified bytes eligible
  for `go-selfupdate`.
- A distinct signed game-file manifest carries the game version and each file's
  relative path, URL, size and SHA-256. The patcher rejects traversal and
  duplicate paths, installs verified files under the user's local app-data game
  root, promotes an immutable version directory, and atomically changes a small
  current-version record while retaining the previous version.
- Failed game downloads do not change the current version. A failed post-update
  health check atomically selects the retained previous version again.
- `TestTwentyInterruptedDownloadsResume` proves 20/20 interrupted transfers
  resume with Range and reproduce the signed bytes. Separate tests cover a
  Range-ignoring server and a corrupted partial file that requires a clean
  verified retry.
- `TestOneThousandManifestMutationsAreRejected` rejects 1,000/1,000 mutations
  across signature, version, size, hash, URL and key ID.
- Launcher and game rollback tests restore the known-good version well under
  the 30-second ceiling. The WP32 unavailable-update invariant remains in the
  test suite: offline play starts without awaiting the background update check.
- Local verification passed `just check`, the focused update and rollback
  tests, 20 repeated runs of the 20/20 unavailable-network launch test, a
  Windows Wails build, and the Dockerized Linux WebKitGTK 4.1 build.

Still open for WP9:

- Delta patching, if release sizes show that it is needed.
- The protected release-environment signing ceremony, which requires human
  approval and real release infrastructure; no private signing material belongs
  in this public repository.
- The three-OS signature matrix, scheduled for WP26.

## Follow-on packages

WP8 must add optional multiplayer without changing the offline default: RFC
8252 loopback PKCE and RFC 8628 device login in Go, memory-only access/ID tokens,
remember-me refresh tokens in the OS keyring with memory fallback, invalid-grant
cleanup and family-wide sign-out, a control-API ticket request, and one
length-delimited `LaunchHandoff` written to game stdin under
`--auth-token-stdin`. Golden fixtures must decode in both Go and Unity C#.
Tokens must remain absent from JavaScript, argv, logs and stdout.

## Session 3: offline installation and protected signing

Delivered in this session:

- Release-mode launcher-manifest URLs, launcher artifact URLs, game-manifest
  URLs and game-file URLs require HTTPS. Plain HTTP remains available only to
  local development and tests. Windows-target game manifests reject
  case-insensitive path collisions, reserved device names with any extension,
  traversal and unsafe separators before any download begins.
- `docs/manifest-signing.md` specifies the exact launcher newline serialization
  and game-manifest canonical JSON field order. `cmd/signmanifest` signs or
  verifies launcher and game manifests with an Ed25519 key supplied at runtime
  through a named environment variable or explicit file path. It never prints
  key material, and production signing refuses all repository test keys.
- The manual-only `release` workflow builds the Wails Windows launcher with
  both injected public keys, prepares unsigned manifests, and passes only the
  signing job through the protected `release` environment. That job installs
  no dependencies, fails clearly when either environment secret is absent,
  signs with distinct launcher/game keys, verifies both results, and uploads
  the launcher plus signed manifests. Workflow permissions are read-only and
  every action is pinned by full commit SHA; no `pull_request` trigger exists.
- Offline play now resolves the patch installer's atomically selected
  `current.json` to the executable in its immutable version directory. The
  `FFRESTART_GAME_PATH` and settings field remain a development override.
  Without an installed version, the UI disables Play and offers **Install or
  Update**, which fetches a bounded signed game manifest when the network is
  available. An in-progress or failed install never joins or blocks the play
  path for an already installed game.
- The TypeScript UI exposes only installed/update status, Install or Update,
  Play offline and the development path override. No credential or token value
  crosses the JavaScript boundary.

Verification for session 3:

- `just check` passes frontend install/typecheck/build, Go vet/tests and all
  configured golangci-lint analyzers.
- Focused tests cover release HTTPS policy, Windows path collisions and device
  names, signing and verification, missing signing secrets, test-key refusal,
  installed-version selection and concurrent install/play isolation.
- The unavailable-network test was re-run 20 times, exercising 400/400
  successful simulated offline launches. Local Windows and Dockerized Linux
  Wails builds passed; both builds are also covered by pull-request CI.
- A diff scan found no private key, PEM block, credential or real signing
  material. The only private keys are deterministic values constructed inside
  `_test.go` files, and every associated public key remains in the release
  denylist.

Still open:

- Delta patching, only if measured release sizes show that it is needed.
- The owner's one-time offline signing ceremony: generate and seal separate
  launcher/game keys, configure the protected environment secrets and public
  key variables, enforce the required reviewer and release-ref rules, and run
  the first approved workflow plus independent verification. Rotation requires
  shipping the next public key before changing the protected secret, retaining
  an overlap window, revoking the old key and completing the WP27 drill.
- WP26's Windows 10/11, Ubuntu 22.04/24.04, Debian 12, SteamOS and macOS 14
  compatibility/signature matrices and real offline packet-capture timings.
- WP8 optional multiplayer login and stdin hand-off. It must preserve this
  offline default and continue to keep tokens out of JavaScript, argv and logs.

WP26 must exercise Windows 10 22H2 and Windows 11, Ubuntu 22.04/24.04, Debian
12, SteamOS Gaming Mode, and macOS 14 on Apple silicon and Intel/Rosetta. It must
cover offline launch with no network/account and multiplayer login/update/play.
macOS needs a universal app, Developer ID signing with hardened runtime,
notarization, stapling, and validation on a dedicated Mac runner.

## Original-launcher experience parity

> **OWNER APPROVAL REQUIRED:** when no signed game manifest is configured, the
> launcher exposes the original **unsigned developer build channel**. It is
> intentionally weaker than D13/WP9's signed design, although it is no weaker
> than the WPF launcher it replaces. Configuring a signed game manifest disables
> this compatibility channel automatically. Offline play never waits for either
> update channel.

The WPF reference for this comparison is the owner's read-only
`0-zach/launcher-auth-ui-integration` branch at `b01c8a3`. Only assets already
public on `origin/main` are used. The private MP3 remains outside Git and is
loaded from the user's local app-data directory.

| Original feature or visual element | Wails result | Notes |
|---|---|---|
| 1100×680 centered launcher and 880×620 minimum | Ported | Native Wails window uses the same dimensions and floor. |
| Void, panel, raised-panel, text, muted, nano-green, cyan and danger palette | Ported | CSS variables preserve the original values and translucent layering. |
| Bahnschrift SemiCondensed display and Segoe UI Variable body typography | Ported | Arial Narrow/Avenir condensed and system UI fallbacks cover Linux and macOS. |
| `newloginbackground.png` scene and horizontal darkening treatment | Ported | Served from an embedded copy of the already-public repository asset. |
| ReStart logo, hero/menu art and public button artwork | Ported | Logo and hero art are visible; button art remains available but CSS provides sharper scalable controls. |
| Cyan/green circuit decoration, shadows and glass-like panels | Ported | Responsive CSS recreates the header, hero field and mission panel. |
| Primary and secondary button states, keyboard focus and disabled states | Ported | Hover, press, focus, busy and disabled treatments are retained. |
| High-contrast cyan-bordered tooltips | Ported | Tooltips use the original dark raised background and readable foreground. |
| Entrance, hover, progress and status animations | Ported | Reduced-motion preferences are respected. |
| System window chrome | Ported | The WPF reference used normal system chrome, so Wails keeps native chrome rather than inventing a frameless title bar. |
| Brand header and guest/signed-in pilot chip | Ported | Account state contains display state only; no credential reaches TypeScript. |
| Hero welcome label and fight-for-the-future headline | Ported | Placement and condensed uppercase treatment match the reference. |
| First-run setup | Ported | A modal explains offline/account behavior and confirms or changes the install root before continuing. |
| Install-location selection, default reset and persistence | Ported | The default is the original `%LOCALAPPDATA%\FFReStart` root. Settings migrate PR 6's exact obsolete `FFReStart\game` default back to that root without changing custom locations. The native chooser starts at the selected directory or its nearest existing parent, so a missing folder cannot break Change. |
| Executable discovery | Ported | Signed installs remain preferred; the exact legacy `FFReStart-Dev-Build\FFReStart-Dev-Build.exe` layout is detected in the chosen root, launched with that folder as its working directory, and falls back to safe bounded discovery. |
| Checking, ready, offline-ready, failure, download and install states | Ported | The mission card, dot, version badge, action label and status copy change together. Existing installs remain playable after update failures. |
| Install/update progress | Ported | The unsigned developer download reports byte progress when GitHub supplies a length; the signed/resumable WP9 channel retains its indeterminate active indicator. |
| Play button retry/install/launch behavior | Changed | Offline play is the primary ready action. When no game exists, the same primary control invokes the signed installer. |
| Offline launch during unavailable updates | Ported | The no-auth, non-blocking path and its 20/20 test remain unchanged. |
| Discord, support and game-files actions | Ported | Discord maps exactly to `Q5je3v9Bjg`, Support to `VNVjmPn2Fn`, and Game Files creates then opens the persisted root with `explorer.exe`, `open`, or `xdg-open` using one explicit argument and no shell. Failures appear in the error banner. |
| Persisted launcher settings | Ported | Versioned JSON is written atomically under the per-user local app-data directory; it contains no credential material. |
| Theme autoplay, loop, volume and mute | Ported | HTML audio uses the persisted 35% default and local-only MP3 endpoint, with a silent missing-file fallback. |
| Embedded private theme asset | Changed | Deliberately excluded. `scripts/install-local-music.ps1` uses read-only `git show` to install it at `%LOCALAPPDATA%\FFReStart\launcher\audio`. |
| Username/password form and remembered local password session | Changed | Replaced per D08/D13 with external-browser S256 PKCE or RFC 8628 device sign-in. The launcher never handles passwords. |
| Keyring-backed remembered session | Ported | Refresh tokens use the OS keyring with process-memory fallback; access and ID tokens remain memory-only in Go. |
| Multiplayer launch after sign-in | Deferred | UI clearly says unavailable until WP7 tickets and WP8 game hand-off exist. Sign-in is optional and never gates offline play. |
| Authentication token passed in argv | Changed | No token is passed today. The approved future path is one length-delimited hand-off over stdin with only `--auth-token-stdin` in argv. |
| Preview-only WPF mode | Deferred | The production Wails layout is directly previewable through the frontend dev server; a separate runtime preview flag adds no user-facing capability. |
| WPF zip updater and version text file | Changed | Restored as an explicitly labelled unsigned developer compatibility channel using the two exact pinned GitHub release URLs. Redirects are restricted to GitHub release-asset hosts; archive and entry sizes are bounded; zip traversal and symlinks are rejected; staging is atomically promoted with a retained `.previous` rollback. `Version.txt` uses the original three-part numeric comparison and is displayed as `vX.Y.Z`. A configured signed manifest disables this channel. |

### Control audit

| Control | Result | Failure or disabled-state communication |
|---|---|---|
| Music mute and volume | Working | Disabled only when the local private track is absent; the panel and setting explain how to install it. Persistence failures use the visible error banner. |
| Browser sign-in and device-code sign-in | Working when configured | Disabled with a tooltip when ZITADEL is unconfigured or the pilot is already signed in. Multiplayer remains labelled unavailable pending WP7/WP8. |
| Sign out | Working | Visible only for a signed-in pilot; keyring failures use the error banner. |
| Play / Install or Update | Working | Plays the detected legacy or signed install offline. With no install it invokes the configured channel. Busy state disables it with a wait tooltip; all failures remain visible. |
| Check for Updates | Working | Uses the signed channel when configured, otherwise the clearly labelled unsigned developer channel. Busy state explains why it is disabled. |
| Discord and Support | Working | Use the original exact mapping; OS-open failures use the error banner. |
| Game Files | Working | Creates and opens the install root, not the build subfolder; OS-open and filesystem failures use the error banner. |
| Default, Change, and setup/settings folder controls | Working | Change uses the nearest existing chooser directory. All location controls are disabled during installation with an explanatory tooltip. |
| First-run Continue | Working | Persists completion; it is disabled during installation and save failures use the error banner. |
| Settings gear and close | Working | Open and close the preferences modal without backend state changes. |
| Autoplay music setting | Working | Disabled with an installation tooltip when the private track is absent; persistence failures use the error banner. |
| Error dismiss | Working | Dismisses the persistent, accessible error banner after the failure has been read. |

Deferred items are deliberately limited to integration work that does not yet
exist upstream: multiplayer tickets/stdin hand-off, signed-channel byte-level
progress, and the multi-OS release/signature matrix already assigned to WP26.

Parity verification on Windows 11 passed `just check`, clean Wails
`windows/amd64` and Dockerized WebKitGTK 4.1 Linux builds, and 20 repeated runs of the 20/20 unavailable-network
offline-launch test (400 simulated launches). A native packaged-app smoke test
rendered the 1100×680 experience with all three public image assets, and its
accessibility state reported audio playing from the private local track. The
owner's executable is `build/bin/ffrestart-launcher.exe`.

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
- PR 6 regression coverage verifies the OS-specific folder commands, nearest
  existing dialog default, stale-default migration, exact legacy layout and
  displayed version, offline legacy launch, developer-channel URL/redirect
  pinning, zip-slip rejection, atomic promotion with retained rollback, and
  signed-manifest precedence. The 20/20 unavailable-network test passed 20
  repeated runs (400 simulated offline launches).
