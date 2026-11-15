# ADR-0010: Portable Data Directory and Automatic Keychain-to-File Token Fallback

- **Status**: accepted

## Context

ADR-0008 put session tokens exclusively in the OS keychain and degraded
loudly when it was unavailable — no silent plaintext fallback. On devices
without a usable Secret Service (common on Linux), that meant re-login on
every launch and lost sessions across restarts. Persisted data lived in
`~/.hrpml` and the default game directory in `~/.minecraft`, so nothing
was portable with the executable. Several settings were volatile or
split-brained: launch settings lived in a localStorage copy that diverged
from the SQLite `settings` table, the selected version and Java runtime
were forgotten on restart, and the login username was never remembered.

Product decisions recorded with the owner (2026-11-15):

- Persisted data **and the default game directory** live next to the
  executable, not in the user's home directory.
- Devices without an OS keychain **fall back automatically** to file-based
  token storage. This deliberately relaxes ADR-0008's no-silent-fallback
  stance for tokens; the rest of ADR-0008 stands.
- All configuration is persisted, including login credentials (tokens;
  plaintext passwords are still never stored).

## Decision

- **Data directory**: `database.DataDir()` defaults to the executable's
  directory when it is writable (temp-file probe), otherwise `~/.hrpml`.
  The result is cached (`sync.OnceValue`); `DataDirFn` is the test seam.
  `AdoptLegacyData` moves `data.db` (with WAL sidecars) and
  `authlib-injector.jar` out of `~/.hrpml` once at startup, never
  overwriting an existing target.
- **Game directory**: the default is `<DataDir>/.minecraft`
  (`GetMinecraftDir`). A persisted user override (`SetGameDir`) wins;
  existing installations elsewhere stay reachable through the
  `ScanGameDirs` disk scan and the Settings UI.
- **Tokens**: `FallbackTokenStore` wraps the keychain store (primary) and
  a `FileTokenStore` (fallback) writing `<DataDir>/tokens.json` — 0700
  directory, 0600 file, atomic temp+rename writes. `Set` falls back when
  the keychain errors and deletes a stale fallback copy once the keychain
  succeeds again; `Get` prefers the keychain, then the file, and reports
  `ErrTokenNotFound` when the fallback is readable but has no entry (the
  user is prompted to re-login, not shown a storage error).
  `KeychainStatus` reports `backend: "keychain" | "file" | ""`; the
  frontend shows an informational callout when the file backend is active.
- **Settings unification**: HomeView reads and write-throughs launch
  settings via `GetLauncherConfig`/`UpdateLauncherConfig` (SQLite
  `settings` table), adopting the old localStorage `launcher_settings`
  copy once. The selected Java id (`selected_java`), selected version
  (`selected_version`) and last login username (`last_username`) persist
  in localStorage as metadata; a restored version that is no longer
  installed is cleared. Account metadata stays in webview localStorage
  (ADR-0008 schema 3, tokens stripped).

## Consequences

- The launcher folder is portable: database, tokens and game directory
  travel with the executable. A read-only executable location falls back
  to `~/.hrpml` automatically.
- `tokens.json` is protected by filesystem permissions only — weaker than
  an OS keychain. The UI discloses the fallback; this is the owner-approved
  trade-off that supersedes ADR-0008's no-fallback stance for tokens.
- Legacy `~/.hrpml` data is adopted once; when both locations hold data,
  the new location wins and the legacy copy is left untouched.
- The account list itself is per-webview (localStorage), not part of the
  portable folder; tokens are portable, account metadata is not.
- Launch settings now require the backend; save failures surface as a
  callout instead of being silently lost in a divergent localStorage copy.

## Evidence

- Data directory + adoption: `internal/database/database.go:26`
  (`DataDirFn` seam), `:35` (`defaultDataDir`), `:46` (`dirWritable`),
  `:58` (`DefaultConfig`), `:69` (`AdoptLegacyData`).
- Token fallback: `internal/services/tokenstore.go:129` (`FileTokenStore`),
  `:254` (`FallbackTokenStore`), `:312` (`Backend`); wiring at `app.go:67`,
  legacy adoption call at `app.go:82`.
- Status binding: `bindings.go:537-554` (`KeychainStatus`, `backend` field).
- Game directory default: `internal/services/gamedir.go:24`
  (`GetMinecraftDir`).
- Frontend: `frontend/src/App.tsx:44` (version storage key), `:98`
  (file-backend notice), `:264` (persisted version selection);
  `frontend/src/components/HomeView.tsx:203` (`loadSettings`), `:232`
  (`persistSettings`); `frontend/src/components/LoginView.tsx:44-47`
  (username prefill).
- Tests: `internal/services/tokenstore_test.go:75-271` (file store
  round-trip/permissions + fallback matrix),
  `internal/database/database_test.go:140-213` (writability probe,
  adoption).