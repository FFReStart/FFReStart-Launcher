# Manifest signing

Release manifests use Ed25519 and two distinct keys: one for launcher updates
and one for game files. `cmd/signmanifest` reads a private or public key only
from the environment-variable name or file path supplied at runtime. It never
prints key material. Production signing rejects every public key in the
repository's test-key denylist; `-development` is required to use test keys or
plain HTTP URLs.

## Exact signed bytes

No outer manifest JSON, whitespace, trailing JSON newline, or `signature` field
is signed.

For a launcher manifest, UTF-8 encode these fields in this exact order, with
one LF byte (`0x0a`) after every field, including `key_id`:

```text
version + "\n" + url + "\n" + base-10 size + "\n" +
lowercase sha256 + "\n" + key_id + "\n"
```

For a game manifest, Go's `encoding/json.Marshal` serializes this exact
anonymous structure. Field order is therefore `version`, `key_id`, `files`;
each file's field order is `path`, `url`, `size`, `sha256`. There is no trailing
newline in the signed bytes.

```go
struct {
    Version string `json:"version"`
    KeyID   string `json:"key_id"`
    Files   []File `json:"files"`
}{manifest.Version, manifest.KeyID, manifest.Files}
```

The signature is raw-standard-base64 encoded in the outer manifest's
`signature` field. File array order is significant. Release manifests and all
artifact URLs must use HTTPS. Windows-target game manifests also reject
case-insensitive path collisions and reserved device names, including device
names followed by an extension.

## Signing and verification

The key value may be raw Ed25519 bytes, hex, or standard/raw-standard base64.
Signing accepts a 32-byte seed or 64-byte private key. Verification accepts a
32-byte public key.

```powershell
go run ./cmd/signmanifest sign -type launcher -manifest launcher.unsigned.json -output launcher.json -key-env LAUNCHER_MANIFEST_PRIVATE_KEY
go run ./cmd/signmanifest verify -type launcher -manifest launcher.json -key-file launcher-public-key.hex
go run ./cmd/signmanifest sign -type game -target-os windows -manifest game.unsigned.json -output game.json -key-env GAME_MANIFEST_PRIVATE_KEY
```

## Owner's one-time ceremony

1. On a dedicated offline machine, generate separate random Ed25519 keys for
   launcher and game manifests. Never use the repository's deterministic test
   seeds and never put private material in this repository, a shell history, a
   workflow input, or a log.
2. Record each 32-byte public key as lowercase hex. Store the encrypted private
   keys in a sealed offline backup held by a second trusted person, then remove
   working copies from the generation media.
3. In GitHub, create or configure the public repository environment named
   `release`. Require the owner as reviewer and restrict deployment to release
   tags/refs. Add `LAUNCHER_MANIFEST_PRIVATE_KEY` and
   `GAME_MANIFEST_PRIVATE_KEY` as environment secrets.
4. Add `UPDATE_PUBLIC_KEY_HEX` and `GAME_PUBLIC_KEY_HEX` as repository
   variables. Build a release only after the public values embedded by
   `cmd/releasebuild` match the private keys in the protected environment.
5. Run the manual **Protected release build and signing** workflow, review the
   unsigned hashes at the environment gate, approve it, then independently
   verify the downloaded signed manifests before promotion.

For rotation, generate and back up a new separate key, ship its public key in a
launcher release verified by the old key, wait for that launcher to be adopted,
then change the protected secret and key ID. Retain the previous verification
key for the declared overlap window, revoke the old key after the window, and
run the WP27 compromise drill. If a key may be exposed, stop releases and use
the offline recovery ceremony; do not silently replace a repository variable.
