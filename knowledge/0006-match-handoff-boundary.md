# ADR-0006: Match Handoff Boundary — Loopback Interop Listener Beside the Wails Binding Surface

- **Status**: accepted

## Context

ADR-0004 retired the frontend-facing Gin HTTP boundary: the frontend reaches
the backend exclusively through Wails bindings, and the process stopped
opening any listener for it. The MatchCore handoff feature (matchmaking
client hands a finished match to SamuelClient, which launches straight into
the match server) needs a cross-process channel: the matchmaking client is a
separate process with no access to Wails bindings.

The ADR-0004 threat model matters here. The retired Gin listener was
dangerous because it was unauthenticated, bound all interfaces, had open
CORS, and exposed write/exec routes to any web page. Any new listener must
not reintroduce those properties.

## Decision

Add a **separate, inter-process-only** loopback boundary, defined by
`INTEROP-SPEC.md` (protocol v1) and implemented in `internal/interop`:

1. **Not a frontend boundary.** The frontend still uses Wails bindings
   exclusively (`GetPendingHandoff`/`DismissHandoff`/`GetInteropStatus` and
   the `interop:handoff` event). The listener serves only the matchmaking
   client on the same machine and user session.
2. **Loopback-only, random port.** Bound to `127.0.0.1:0`; never `0.0.0.0`,
   no fixed port to collide on.
3. **Per-process bearer token in a 0600 discovery file**
   (`$XDG_RUNTIME_DIR/matchcore-interop/samuel-client.json`, directory
   0700), carrying `app/protocol_version/pid/port/token`. Readers must
   pid-check before trusting it; the file is removed on clean exit.
4. **No credentials cross the boundary.** Handoff payloads are validated
   against a strict schema (format/length limits) and a credential firewall
   rejects any payload containing `access_token`/`password`/`token`-like
   keys. SamuelClient launches with its own logged-in account.
5. **Deep links carry no secrets.** `matchcore://join?handoff_id=…&v=1` is
   an invocation hint; the real parameters are pulled over the loopback
   channel afterwards. Linux `.desktop` + `x-scheme-handler` registration is
   implemented; Windows/macOS are documented gaps (Linux-first).
6. **Readable degradation everywhere**: version mismatch, expired/gone
   handoffs, unreachable match server (pre-launch TCP dial), and missing
   game versions all surface messages; the matchmaking result is never
   dropped silently. Optional one-time nonces guard against replay.
7. **No result reporting.** Match results remain the BedwarsRel plugin's
   job; this boundary only delivers the player to the server.

## Consequences

- The process opens a listener again, but with none of the retired
  boundary's properties: loopback bind, strong random auth, same-user file
  permissions, no browser reachability (no CORS surface, and web pages
  cannot read the 0600 token file), tiny validated schema instead of
  arbitrary routes.
- The binding surface grew by three read/write-neutral methods
  (`GetPendingHandoff`, `DismissHandoff`, `GetInteropStatus`) and one event
  (`interop:handoff`), documented in README and `bindings.go`.
- `LaunchGame` accepts optional `join_server`/`join_port` (validated,
  preflight-dialed, never persisted); `BuildLaunchCommand` maps them to
  `--server`/`--port`.
- Costs/limits: single handoff slot (concurrent pushes get `503 busy`);
  deep-link wake-up depends on desktop registration succeeding
  (best-effort, logged); off Linux the cold-start flow is unavailable until
  the documented registration gaps are closed.

## Evidence

- `internal/interop/` — protocol types/validation (`interop.go`), discovery
  files (`discovery.go`), loopback server (`server.go`), deep-link parsing
  and Linux registration (`deeplink.go`, `deeplink_linux.go`), pull client
  (`client.go`); all covered by hermetic tests (`*_test.go`).
- `app.go` — listener lifecycle, single handoff slot, deep-link pull loop,
  `interop:handoff` emission; `main.go` — Wails single-instance lock and
  argv deep-link intake.
- `bindings.go` — handoff bindings, `LaunchGame` join-target preflight,
  event contract header.
- `internal/services/mc_services.go` — `--server`/`--port` emission in
  `BuildLaunchCommand` (`TestBuildLaunchCommandJoinTarget`).
- Frontend: `src/components/HandoffBanner.tsx` (+tests), `src/App.tsx`
  auto-resume after login, `src/i18n.ts` English catalog.
- Verified on branch `feat/match-handoff-receiver`: `go build/vet/test
  -race` green, `npm run lint` 0/0, `npm test` green, `npm run build`
  green. Real cross-process runs with the actual matchmaking client: not
  verified (sender side is not built yet).
