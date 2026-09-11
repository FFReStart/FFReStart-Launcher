# Launcher authentication

## Existing account authority

The game currently has no remote authentication API, access token, refresh token, or HTTP endpoint. Its canonical `AccountService` is a local account database at:

`%USERPROFILE%\AppData\LocalLow\FFReStart-Unity\FFReStart-Dev-Build\Accounts\accounts.json`

Usernames are trimmed and lower-cased. Passwords are verified with PBKDF2-HMAC-SHA256, a 16-byte salt, 100,000 iterations, and a 32-byte result. The launcher reads this database and performs the same fixed-time verification. It does not write the account database or retain the password. Create a local account through the game's normal login screen before using launcher sign-in.

There is no server refresh operation. A remembered launcher session is revalidated against the current account record at startup and immediately before every launch. Removing the account or changing its password invalidates the session.

## Game launch contract

The launcher starts the selected executable without a shell and adds two structured arguments through `ProcessStartInfo.ArgumentList`:

```text
Game.exe --auth-token <token>
```

The token is a five-minute HMAC-SHA256 ticket:

```text
v1.<subject>.<issued-at>.<expires-at>.<nonce>.<signature>
```

- `subject` is Base64URL without padding of the first 16 bytes of `HMAC-SHA256(accountHash, ASCII("FFReStart.AuthTicket.Subject.v1"))`. It is an opaque account selector and does not contain the username.
- `issued-at` and `expires-at` are Unix seconds. Issued tickets last exactly five minutes; the game permits at most 30 seconds of clock skew.
- `nonce` is 16 cryptographically random bytes encoded as unpadded Base64URL.
- `signature` is unpadded Base64URL of HMAC-SHA256 over the first five ASCII segments joined by `.`. The HMAC key is the decoded 32-byte PBKDF2 hash already held in the matching account record.

The game must validate the ticket and initialize its normal account session before bypassing interactive login. Malformed, expired, unknown, or incorrectly signed tickets fall back to normal game login.

## Remember Password security

“Remember Password” is optional and off by default. Despite the UI wording, the launcher never persists the password. It stores only the minimum derived session material needed to mint a new short-lived ticket: the normalized account identity and the current 32-byte account verifier.

The entire payload is protected using Windows DPAPI with `CurrentUser` scope and launcher-specific optional entropy. The resulting opaque blob is written atomically with an ACL restricted to the current Windows user:

```text
%LOCALAPPDATA%\FFReStart\Launcher\auth-session.dat
```

The optional executable override is likewise stored as a DPAPI-protected blob in `settings.dat`, rather than plaintext configuration. Unchecking Remember Password, signing out, a failed login, account removal/password change, corrupt or truncated state, a profile migration that prevents decryption, or an unsupported OS crypto provider invalidates and deletes remembered state. To clear it manually, sign out or delete `auth-session.dat` while the launcher is closed.

Passwords are copied from WPF `SecureString` into a mutable character buffer only for PBKDF2 and cleared in a `finally` block. Passwords are never passed to the game, logged, or placed in configuration.

DPAPI protects data at rest from other Windows users and offline inspection. It does not protect against malware or an attacker already running as the same Windows user, who may be able to invoke user-scoped decryption or inspect process memory.

## Command-line exposure

The token is necessarily visible to local process inspection because the integration contract uses a command-line argument. The launcher limits exposure by issuing it only after an explicit Play action, making it opaque and valid for five minutes, never persisting it, never including it in errors or logs, and releasing launcher references immediately after process start. The game side must redact both `--auth-token <value>` and `--auth-token=<value>` from crash reports.
