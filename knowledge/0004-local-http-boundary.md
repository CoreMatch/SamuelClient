# ADR-0004: Local HTTP Boundary — Retire Direct Frontend→Gin HTTP in Favor of Wails Bindings

- **Status**: accepted
- **Date**: 2026-11-04

## Context

SamuelClient's React frontend did not use Wails bindings for business
functionality; every view called the embedded Gin server directly over plain
HTTP, and the process opened a network listener for it. The author's
assessment: "这个现状需要一个更加安全和便利的替代" — this arrangement needs a
more secure and more convenient replacement. Verified problems with it:

1. **No caller authentication at all.** No token, session, or origin check on
   any route.
2. **The listener bound all network interfaces**, so the API was reachable
   from the LAN, not just the local machine.
3. **CORS was fully open** (`Access-Control-Allow-Origin: *` with credentials
   and all methods allowed). Combined with (1), any web page the user visited
   in any browser could issue cross-origin requests to the launcher API and
   read the responses (drive-by calls).
4. **The exposed surface included arbitrary code execution and file writes**,
   including routes with no frontend caller at all — pure attack surface.
5. **Credentials crossed the wire in clear text**: login POSTed username +
   plaintext password, and launch sent the Yggdrasil
   `access_token`/`client_token`.
6. **The Wails binding layer existed but was a demo**: a handful of methods
   consumed by a single debug view, most of them merely proxying back into Gin
   over HTTP.

Threat model summary: any local process, and any web page rendered in any
browser on the machine, could achieve remote-code execution on the launcher
machine with a single POST; LAN devices could do the same.

## Decision

Adopt **Option A**: move all frontend↔backend communication onto Wails
bindings and retire the frontend-facing Gin HTTP boundary. The owner confirmed
the following on 2026-11-04:

1. **Gin is retired.** All business functionality moves to Wails bindings; the
   HTTP listener is removed, and the README's "Local HTTP API" feature claim
   with it. No external tooling consumed `:34501` (verified in-repo).
2. **Interim hardening lands first, separately.** Deleting the caller-less
   shell/file routes and switching the listener to loopback with the CORS
   wildcard removed are committed independently before the migration, so each
   can be reverted on its own.
3. **Streaming uses Wails Events, not HTTP polling.** Live launch logs and
   download progress are pushed from Go via `runtime.EventsEmit` and consumed
   by the frontend via `EventsOn`. The event contract is fixed here so later
   code never has to guess the interface:
   - `launcher:log` — one event per output line of the launched game process,
     or of the Forge installer while `InstallForgeLoader` runs. Payload:
     `{"source": "stdout" | "stderr", "line": string}`. For the game process,
     emission starts after `LaunchGame` returns success and ends when the
     process exits.
   - `download:progress` — progress while `DownloadVersion` installs a
     version or `InstallFabricLoader`/`InstallForgeLoader` install a mod
     loader. Payload: `{"version_id": string, "stage": "jar" | "library" |
     "native" | "asset" | "loader", "name": string, "downloaded": number,
     "total": number}`. Within `DownloadVersion` the stages arrive in the
     order `jar` → `library` → `native` → `asset`; the `loader` stage belongs
     to the loader installs. The jar stage emits as download chunks arrive;
     library/native/asset emit once per completed artifact; the loader stage
     emits once per completed loader library (Fabric) or once before the
     installer download (Forge).

   The same contract text lives in the comment at the top of `bindings.go`
   and in the README.
4. **`SERVER_PORT` is removed, not preserved.** The `internal/config` package
   (`ServerPort`, `GinMode`) is deleted with the HTTP boundary; no port
   configuration is kept or reserved.

## Options Considered

### Option A — All Wails bindings (Gin retired to internal implementation or removed)

The frontend calls only generated binding methods; App methods call the
existing services/repository layer directly; the Gin listener is removed (or
kept only if a future ADR justifies an external API).

- **Security gain**: highest. No listening socket means no drive-by web attacks,
  no LAN exposure, no cross-process HTTP calls at all; passwords and tokens
  never touch a network stack. The IPC boundary becomes the Wails webview,
  which only the application itself controls.
- **Change scope**: Go — binding methods wrapping the existing
  services/repository layer; Frontend — fetch call sites replaced with binding
  imports; Build — regenerate `frontend/wailsjs` (already gitignored and
  CLI-generated).
- **Migration cost**: medium but mechanical; binding methods are thin wrappers
  over the services layer, so no business logic is rewritten.
- **Platform differences**: none material — Wails bindings behave identically
  on Linux (WebKitGTK 2.41+), Windows (WebView2), and macOS (WKWebView), all
  already required by this project. Removing the listener additionally avoids
  OS firewall prompts for desktop apps (general behavior, not verified for this
  app specifically) and keeps future macOS sandboxing feasible.
- **Known risks**: breaks any hypothetical external consumer of the HTTP API
  (none was found in-repo). Binding surface becomes the only API, so its
  signatures must be reviewed as seriously as HTTP routes.

### Option B — Keep local HTTP, add one-time handshake credential + Origin/Referer checks + loopback-only listener

Gin binds `127.0.0.1` on a (possibly dynamic) port; the Go side generates a
random session token at startup and hands it to the webview through a Wails
binding; the frontend sends the token in a request header; middleware rejects
requests lacking the token, and additionally enforces an Origin allowlist for
browser callers. CORS `*` is removed.

- **Security gain**: medium-high. Loopback-only kills LAN exposure; the token
  blocks other local processes that cannot read it; the Origin check blocks
  browser drive-by requests (Origin is reliable for cross-origin fetch; Referer
  alone is not). Residual risk: the token lives in webview JS memory, so any
  code executing inside the webview (e.g., a compromised npm dependency or XSS)
  holds full API access; non-browser attackers can set any Origin header, so
  the token is the real gate and Origin is defense-in-depth only.
- **Change scope**: Go — token generation, auth middleware, listener address,
  CORS tightening. Frontend — fetch the token via one binding call, add a
  header to every fetch. Build — unchanged.
- **Migration cost**: low; call sites change by one shared header, routes stay.
- **Platform differences**: loopback binding is uniform across
  Linux/Windows/macOS. A fixed port still risks collisions with other local
  software; a dynamic port requires token+port discovery, which itself needs a
  binding — at which point half of Option A is already built.
- **Known risks**: port remains scannable and probe-able; token leakage (logs,
  devtools, crash dumps) restores full access; two authorization stories
  (browser Origin vs. process token) are easy to misconfigure; the fundamental
  "any local web page is one CORS misconfiguration away from RCE" shape
  remains.

### Option C — Hybrid: sensitive operations via bindings, non-sensitive reads via token-protected HTTP

Auth (login/refresh/validate), launch, download, and all DB writes move to
Wails bindings; read-only listings (versions, fabric/forge, installed versions,
java list, logs) stay on HTTP behind the Option-B token. Shell/file routes are
deleted outright.

- **Security gain**: medium-high. Credentials and tokens never traverse HTTP;
  the RCE-grade endpoints disappear from the network surface; remaining HTTP
  endpoints are low-value reads, gated by the token.
- **Change scope**: Go — new bindings for sensitive ops + token middleware +
  route deletion. Frontend — split refactor: LoginView/HomeView launch and
  SettingsView writes onto bindings; list views keep fetch.
- **Migration cost**: medium, with ongoing cost: two IPC paradigms coexist, and
  every future endpoint needs a "which side?" judgment that will drift.
- **Platform differences**: union of A and B; no additional platform concerns.
- **Known risks**: boundary ambiguity over time (what counts as "sensitive"
  erodes); duplicated plumbing and testing; the drive-by window stays open for
  the HTTP subset, so the security story is permanently partial.

### Comparison Summary

| Criterion | A (all bindings) | B (hardened HTTP) | C (hybrid) |
|---|---|---|---|
| Drive-by web RCE eliminated by construction | yes | no (token-gated) | partially |
| LAN exposure removed | yes | yes (loopback) | yes (loopback) |
| Credentials leave the process | never | yes (HTTP body) | never for auth, yes elsewhere |
| Migration cost | medium | low | medium |
| Ongoing maintenance | one IPC path | one IPC path + token lifecycle | two IPC paths |
| External HTTP consumers still served | no | yes | partially |
| Platform deltas (Linux/Win/macOS) | none | none | none |

## Recommendation

**Option A.** SamuelClient is a single-user desktop launcher; no verified
in-repo consumer needs an HTTP API. Option A is the only option that removes
the drive-by RCE shape by construction rather than by a secret that must be
kept safe forever, it simplifies the codebase to one IPC paradigm, and it has
no platform-specific cost. If an external HTTP API is ever genuinely needed
(CLI tooling, CMCL-style integration), a future ADR can reintroduce it under
Option-B controls — deliberately, not by default.

## Consequences

- The attack surface is "the application's own webview IPC" rather than an
  unauthenticated LAN-wide HTTP service with shell execution: the process
  opens no listening port, and credentials never cross a network stack.
  Frontend and backend are build-time coupled through generated bindings
  (already the Wails default).
- Cost: any future external integration must be designed deliberately rather
  than reusing an accidental API.

## Follow-up Decisions (2026-11-05)

After the migration landed, the owner decided a second round of reductions:

1. **The `logs` table and `GetLogs` are retired.** Live launch logs stream
   over `launcher:log` events instead; nothing ever wrote to the table.
   **Existing deployments**: no `DROP TABLE` is issued on purpose — an already
   created `logs` table stays inert on disk (never read, never written, never
   created again). A destructive migration was judged to cost more (irreversible
   data deletion, upgrade-path risk) than it could ever save, since no code
   path inserts into the table.
2. **DebugView removed.** The binding surface is exactly the launcher business
   methods; no debug-only bindings remain.
3. **Two intentional behavior fixes from the migration — do not treat them as
   suspicious drift**:
   - LoginView reads the custom auth-server name from `meta.serverName` (the
     documented shape of `AuthService.GetAuthlibMeta`); the old code read a
     top-level `serverName` that was always undefined, and the fetch itself
     only ever worked in dev mode.
   - `clientToken` generation now uses `crypto/rand`
     (`bindings.go::newClientToken`); the old HTTP handler produced a
     deterministic, non-random placeholder string.

## Evidence

- No HTTP listener: `go.mod` (direct dependencies are Wails and SQLite only);
  `app.go:13-16` (boundary note); `main.go:19-31` (`wails.Run` with the asset
  server and bindings only).
- Binding surface: `bindings.go` (every bound method; the event-contract
  comment at `bindings.go:11-24`); `main.go:28-30` (Bind); generated bindings
  are not committed (`.gitignore`, `frontend/wailsjs/`).
- Event contract consumers: `bindings.go::emit` (`EventsEmit` wrapper),
  `frontend/src/components/HomeView.tsx:132` (`launcher:log`),
  `frontend/src/components/VersionManager.tsx:68` (`download:progress`); the
  same contract text lives in `README.md:99-106`.
- Logs retirement: `internal/database/database.go:100-104` (fresh databases
  create no `logs` table; existing ones keep it inert), asserted by
  `internal/database/database_test.go:73-84`.
- Not verified: none material.
