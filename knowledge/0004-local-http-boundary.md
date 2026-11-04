# ADR-0004: Local HTTP Boundary — Retire Direct Frontend→Gin HTTP in Favor of Wails Bindings

- **Status**: accepted
- **Date**: 2026-11-04

## Context

SamuelClient's React frontend does not use Wails bindings for business
functionality. Instead, every view calls the embedded Gin server directly over
plain HTTP at a hardcoded `localhost:34501` (see Evidence). The author's
assessment: "这个现状需要一个更加安全和便利的替代" — this arrangement needs a
more secure and more convenient replacement. Verified problems with the status
quo:

1. **No caller authentication at all.** The Gin engine installs only Recovery,
   Logger, and CORS middleware; there is no token, session, or origin check on
   any route (`internal/routes/routes.go:L24-L26 @ 27745e6`).
2. **The server binds all network interfaces.** It listens on `":34501"`
   (`app.go:L58 @ 27745e6`), not `127.0.0.1:34501`, so the API is reachable
   from the LAN, not just the local machine.
3. **CORS is fully open.** `Access-Control-Allow-Origin: *` with credentials
   and all methods allowed, and OPTIONS short-circuits to 204
   (`internal/middleware/middleware.go:L32-L46 @ 27745e6`). Combined with
   (1), any web page the user visits in any browser can issue cross-origin
   requests to the launcher API and read the responses (drive-by calls).
4. **The exposed surface includes arbitrary code execution and file writes.**
   `POST /api/v1/shell/execute` and legacy `POST /shell` run arbitrary shell
   commands (`internal/routes/routes.go:L51-L54 @ 27745e6`,
   `internal/routes/routes.go:L202 @ 27745e6`,
   `internal/handlers/handlers.go:L64-L111 @ 27745e6`);
   `POST /api/v1/file/operation` and legacy `POST /file`
   create/delete/overwrite arbitrary files
   (`internal/routes/routes.go:L57-L60 @ 27745e6`,
   `internal/routes/routes.go:L203 @ 27745e6`,
   `internal/handlers/handlers.go:L114-L155 @ 27745e6`). Neither endpoint has
   any frontend caller — they are pure attack surface.
5. **Credentials cross the wire in clear text.** The login flow POSTs
   username + plaintext password to `/api/v1/auth/login`
   (`frontend/src/components/LoginView.tsx:L157-L165 @ 27745e6`,
   `internal/handlers/mc_handlers.go:L57 @ 27745e6`,
   `internal/handlers/mc_handlers.go:L61-L78 @ 27745e6`), and the launch flow
   sends the Yggdrasil `access_token`/`client_token` to
   `/api/v1/launcher/launch`
   (`frontend/src/components/HomeView.tsx:L273-L294 @ 27745e6`,
   `internal/handlers/mc_handlers.go:L424-L425 @ 27745e6`). Harmless on true
   loopback, but compounded by (2).
6. **The Wails binding layer exists but is a demo.** Only six methods are bound
   (`Greet`, `TestHealthCheck`, `TestPing`, `TestAPICall`, `GetDBPath`,
   `GetDBSchema`; `app.go:L62-L113 @ 27745e6`), the only frontend consumer is
   DebugView (`frontend/src/components/DebugView.tsx:L4 @ 27745e6`,
   `frontend/src/components/DebugView.tsx:L24 @ 27745e6`,
   `frontend/src/components/DebugView.tsx:L45 @ 27745e6`,
   `frontend/src/components/DebugView.tsx:L66 @ 27745e6`), and all but two of
   the bound methods merely proxy back into Gin over HTTP
   (`app.go:L29 @ 27745e6`, `app.go:L67-L103 @ 27745e6`,
   `app.go:L111-L113 @ 27745e6`).
7. **Fragile dev/prod split.** One call uses a relative URL
   (`frontend/src/components/LoginView.tsx:L87 @ 27745e6`) that only resolves
   in dev mode through the Vite proxy
   (`frontend/vite.config.ts:L7-L12 @ 27745e6`); the production asset server
   has no proxy configured (`main.go:L23-L25 @ 27745e6`), so that call is
   expected to fail in packaged builds (production behavior not verified).

Threat model summary: any local process, and any web page rendered in any
browser on the machine, can currently achieve remote-code execution on the
launcher machine with a single POST; LAN devices can do the same.

## Decision

Adopt **Option A**: move all frontend↔backend communication onto Wails
bindings and retire the frontend-facing Gin HTTP boundary. The owner confirmed
the following on 2026-11-04:

1. **Gin is retired.** All business functionality moves to Wails bindings; the
   HTTP listener is removed. The README's "Local HTTP API" feature claim is
   deleted along with it. No external tooling consumes `:34501` (verified
   in-repo; see Evidence).
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

   Contract extensions after the 2026-11-04 decision (both 2026-11-05): the
   `asset` stage was added when asset downloads were wired into
   `internal/services/mc_services.go::InstallVersion`, and the `loader` stage
   with `bindings.go::InstallFabricLoader` /
   `bindings.go::InstallForgeLoader`; `launcher:log` was extended to the
   Forge installer's output. The same contract text lives in the comment at
   the top of `bindings.go` and in the README.
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
- **Change scope**: Go — add App methods wrapping existing handlers/services
  (auth meta/login/refresh/validate, versions, fabric/forge, download, launch,
  launcher config get/put, java CRUD, logs); delete or internalize routes,
  middleware, and the HTTP client service. Frontend — replace ~20 fetch call
  sites across 5 components with binding imports. Build — regenerate
  `frontend/wailsjs` (already gitignored and CLI-generated).
- **Migration cost**: medium but mechanical; handlers are thin wrappers over
  services, so binding methods reuse the same logic. Streaming-style responses
  (the spawn-shell chunked path,
  `internal/handlers/handlers.go:L88-L111 @ 27745e6`) would need Wails runtime
  events if ever required by the UI; today no frontend code calls those
  endpoints, so nothing is lost.
- **Platform differences**: none material — Wails bindings behave identically
  on Linux (WebKitGTK 2.41+), Windows (WebView2), and macOS (WKWebView), all
  already required by this project. Removing the listener additionally avoids
  OS firewall prompts for desktop apps (general behavior, not verified for this
  app specifically) and keeps future macOS sandboxing feasible.
- **Known risks**: breaks any hypothetical external consumer of the HTTP API
  (README advertises "Local HTTP API" as a feature; the dead `shellApiUrl`
  constant in `frontend/src/variables.ts:L14 @ 27745e6` hints at a historical
  CMCL integration — no live consumer found in-repo). Binding surface becomes
  the only API, so its signatures must be reviewed as seriously as HTTP routes.

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
in-repo consumer needs an HTTP API (the only traces are a README feature
bullet and one dead constant). Option A is the only option that removes the
drive-by RCE shape by construction rather than by a secret that must be kept
safe forever, it simplifies the codebase to one IPC paradigm, and it has no
platform-specific cost. If an external HTTP API is ever genuinely needed
(CLI tooling, CMCL-style integration), a future ADR can reintroduce it under
Option-B controls — deliberately, not by default.

## Rollout Steps (each independently verifiable)

1. **Interim hardening, part 1 — delete dead dangerous routes.** Remove
   `/api/v1/shell/execute`, `/api/v1/file/operation`, and their legacy
   `/shell`, `/file` aliases (no frontend callers exist). Verify: app builds,
   frontend unaffected, endpoints return 404.
2. **Interim hardening, part 2 — loopback + CORS tightening.** Bind
   `127.0.0.1` and restrict CORS to the app's own origin (or drop the CORS
   header entirely once step 5 lands). Verify: frontend still works; the port
   is unreachable from another machine on the LAN.
3. **Read-path bindings.** Add binding methods for read-only operations (auth
   meta, versions, fabric/forge, installed versions, java list, launcher
   config, logs) and migrate HomeView, VersionManager, SettingsView reads,
   including the dev-only relative fetch in LoginView. Verify per view against
   the same scenarios that work today.
4. **Write/sensitive-path bindings.** Add binding methods for login/refresh/
   validate, launcher config update, java CRUD, version download, and launch;
   migrate LoginView, SettingsView writes, VersionManager download, and
   HomeView launch. Verify: full login → download → launch end-to-end on at
   least one platform.
5. **Retire the HTTP boundary.** Remove the Gin startup, routes, middleware,
   and the App's HTTP client; repoint DebugView's bound methods to services
   directly; update the README feature list and knowledge entry 0001. Verify:
   no process-owned listening port remains (e.g., via the platform's socket
   listing tool), and the full end-to-end flow still passes.
6. **Cleanup.** Delete `frontend/src/variables.ts` dead constants if still
   unreferenced; record the outcome in this knowledge base (flip this entry to
   `accepted`, or supersede it if the owner chooses B/C).

## Open Questions for the Owner

1. Is retiring the Gin HTTP API acceptable? README lists "Local HTTP API" as a
   feature; is anything outside this repository (scripts, CMCL, other tools)
   calling `:34501`? If yes, Option C or a governed Option-B revival is needed
   instead.
2. May the interim hardening (steps 1–2) land immediately, ahead of the
   migration decision? They are safe regardless of which option is chosen.
3. Is there any plan for streaming UI (live launch logs, download progress)
   that would require Wails runtime events in step 4? Today the launch endpoint
   returns a single JSON response, so this is not currently needed.
4. After step 5, the `SERVER_PORT` environment variable becomes meaningless;
   is removing it acceptable, or should it be kept reserved for a future API?

All four questions were answered by the owner on 2026-11-04: (1) yes, retire
Gin — nothing outside the repo consumes `:34501`; (2) yes, land the interim
hardening first as separate commits; (3) yes, build the streaming UI on Wails
Events (`runtime.EventsEmit` + frontend subscription, no HTTP polling);
(4) remove `SERVER_PORT` entirely. The decisions are recorded in the Decision
section above.

## Consequences

- If accepted: the attack surface shrinks from "unauthenticated LAN-wide HTTP
  service with shell execution" to "the application's own webview IPC"; the
  codebase loses one whole transport layer (routes/middleware/HTTP client);
  frontend and backend become build-time coupled through generated bindings
  (already the Wails default).
- Cost: any future external integration must be designed deliberately rather
  than reusing an accidental API; the README's "Local HTTP API" feature bullet
  disappears.
- If rejected in favor of B or C: the token lifecycle, Origin allowlist, and
  loopback binding become permanent security-critical configuration that must
  be maintained and reviewed with every route change.

## Follow-up Decisions (2026-11-05)

After the migration landed, the owner decided a second round of reductions:

1. **`express`/`cors` npm packages removed.** They were never imported by the
   frontend (leftover from the retired local-HTTP design); `package.json` and
   the lockfile were regenerated and the frontend build re-verified.
2. **The `logs` table and `GetLogs` are retired.** Nothing ever wrote to the
   table — live launch logs stream over `launcher:log` events instead. The
   `CREATE TABLE`/index migrations, `LogRepository`, the `Log` model and
   `LogLevel` constants, the `GetLogs` binding, and the HomeView Refresh/
   DB-log panel are deleted, together with the orphaned `Stats`/`Cleanup`/
   `Backup`/`GetDBStats` helpers that still referenced the table.
   **Existing deployments**: no `DROP TABLE` is issued on purpose — an already
   created `logs` table stays inert on disk (never read, never written, never
   created again). A destructive migration was judged to cost more (irreversible
   data deletion, upgrade-path risk) than it could ever save, since no code
   path inserts into the table.
3. **DebugView removed**, together with everything that only served it: the
   `GetDBPath`/`GetDBSchema` bindings and the `database.GetSchema` helper, the
   Debug navigation entry/route, and the README mentions. The binding surface
   is now exactly the launcher business methods.
4. **Two intentional behavior fixes from the migration — do not treat them as
   suspicious drift**:
   - LoginView reads the custom auth-server name from `meta.serverName` (the
     documented shape of `AuthService.GetAuthlibMeta`); the old code read a
     top-level `serverName` that was always undefined, and the fetch itself
     only ever worked in dev mode.
   - `clientToken` generation now uses `crypto/rand`; the old HTTP handler
   produced a deterministic, non-random placeholder string.

## Evidence

- All-interfaces listener: `app.go:L55-L59 @ 27745e6`
  (`Run(":" + a.config.ServerPort)`); default port `34501`,
  `internal/config/config.go:L17 @ 27745e6`.
- Middleware set (no auth): `internal/routes/routes.go:L24-L26 @ 27745e6`;
  open CORS, `internal/middleware/middleware.go:L32-L46 @ 27745e6`.
- Shell/file execution surface: `internal/routes/routes.go:L51-L54 @ 27745e6`,
  `internal/routes/routes.go:L57-L60 @ 27745e6` (v1 routes) and
  `internal/routes/routes.go:L197-L205 @ 27745e6` (legacy); handlers
  `internal/handlers/handlers.go:L64-L111 @ 27745e6` (shell, incl. chunked
  spawn `internal/handlers/handlers.go:L88-L111 @ 27745e6`),
  `internal/handlers/handlers.go:L114-L155 @ 27745e6` (file operations).
- Direct frontend fetches to `localhost:34501`:
  `frontend/src/components/LoginView.tsx:L157-L165 @ 27745e6` (login incl.
  plaintext password),
  `frontend/src/components/VersionManager.tsx:L92 @ 27745e6`,
  `frontend/src/components/VersionManager.tsx:L119 @ 27745e6`,
  `frontend/src/components/HomeView.tsx:L112 @ 27745e6`,
  `frontend/src/components/HomeView.tsx:L142 @ 27745e6`,
  `frontend/src/components/HomeView.tsx:L190-L191 @ 27745e6`,
  `frontend/src/components/HomeView.tsx:L226 @ 27745e6`,
  `frontend/src/components/HomeView.tsx:L273-L294 @ 27745e6` (launch body
  carries `access_token`/`client_token`),
  `frontend/src/components/SettingsView.tsx:L25 @ 27745e6`,
  `frontend/src/components/SettingsView.tsx:L66 @ 27745e6`,
  `frontend/src/components/SettingsView.tsx:L85 @ 27745e6`,
  `frontend/src/components/SettingsView.tsx:L138 @ 27745e6`,
  `frontend/src/components/SettingsView.tsx:L188 @ 27745e6`,
  `frontend/src/components/SettingsView.tsx:L216 @ 27745e6`,
  `frontend/src/components/SettingsView.tsx:L244 @ 27745e6`,
  `frontend/src/components/SettingsView.tsx:L261 @ 27745e6`;
  display-only base URL `frontend/src/components/DebugView.tsx:L19 @ 27745e6`.
- Dev-only relative fetch:
  `frontend/src/components/LoginView.tsx:L87 @ 27745e6` with Vite proxy
  `frontend/vite.config.ts:L7-L12 @ 27745e6`; production asset server has no
  proxy (`main.go:L23-L25 @ 27745e6`) — production behavior not verified.
- Binding layer: `main.go:L28-L30 @ 27745e6` (Bind),
  `app.go:L62-L113 @ 27745e6` (six methods;
  `TestHealthCheck`/`TestPing`/`TestAPICall`/`GetDBSchema` proxy into Gin via
  `app.go:L29 @ 27745e6`, `app.go:L67-L103 @ 27745e6`,
  `app.go:L111-L113 @ 27745e6`); sole consumer
  `frontend/src/components/DebugView.tsx:L4 @ 27745e6`,
  `frontend/src/components/DebugView.tsx:L24 @ 27745e6`,
  `frontend/src/components/DebugView.tsx:L45 @ 27745e6`,
  `frontend/src/components/DebugView.tsx:L66 @ 27745e6`; bindings are
  generated, not committed (`.gitignore`, `frontend/wailsjs/`).
- No frontend callers of shell/file/echo/sysinfo routes or of the `Greet` /
  `TestAPICall` bindings: grep over `frontend/src` returns zero matches.
- Dead external-integration trace: `frontend/src/variables.ts:L14 @ 27745e6`
  (`shellApiUrl`) is exported but imported nowhere.
- Sensitive-data endpoints: `internal/handlers/mc_handlers.go:L57 @ 27745e6`,
  `internal/handlers/mc_handlers.go:L61-L78 @ 27745e6` (login),
  `internal/handlers/mc_handlers.go:L83-L103 @ 27745e6` (refresh),
  `internal/handlers/mc_handlers.go:L110-L128 @ 27745e6` (validate),
  `internal/handlers/mc_handlers.go:L424-L425 @ 27745e6`,
  `internal/handlers/mc_handlers.go:L441-L517 @ 27745e6` (launch with tokens).
- Not verified: whether the packaged production build actually fails the
  relative `/api/v1/auth/meta` call (no packaged build was executed); whether
  any out-of-repo tool calls `:34501` (cannot be proven from this repository);
  OS firewall prompt behavior on Windows for the current listener.
