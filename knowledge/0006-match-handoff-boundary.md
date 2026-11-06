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

## Cross-process verification (INTEROP-SPEC v1)

Real end-to-end run against the actual sender implementation
(MatchCoreArena-Client `836b720`, its `internal/interop`), executed on the
Fedora 44 / KDE / Wayland host. The sender ran from a throwaway `/tmp`
clone so the MCA repository was never modified; the driver source is kept
in this repo at `tools/e2e-handoff-sender/main.go` (Go's internal-package
rule requires compiling it inside the MCA module).

Commands:

```sh
# receiver — real production binary
wails build -tags webkit2_41
setsid env DISPLAY=:0 ./build/bin/SamuelClient > /tmp/e2e-receiver.log 2>&1 < /dev/null &

# sender — inside the /tmp clone of MatchCoreArena-Client @ 836b720
go run ./tools/e2e-handoff-sender   # join_target 127.0.0.1:9 (deliberately unreachable)
```

Observed:

- **Flow A (receiver running):** the sender read `samuel-client.json`,
  passed the pid check, and `POST /v1/join` succeeded — sender states
  `delivering → delivered`; receiver log
  `[interop] handoff accepted: cf4bba01-… match=m-90001 target=127.0.0.1:9`
  (pending slot filled, `interop:handoff` event emitted).
- **Flow B (receiver not running):** sender states
  `delivering → waiting_receiver → delivered`. The deep link
  `matchcore://join?handoff_id=ea9c240d-…&v=1` went through `xdg-open`,
  KDE started SamuelClient via the registered `.desktop` entry (new pid,
  new listener port), and the receiver's pull loop fetched the payload
  from the sender's `GET /v1/handoff/{id}` — receiver log
  `[interop] handoff pulled from sender: ea9c240d-… match=m-90001
  target=127.0.0.1:9` (handoff id identical to the sender's).
- **Busy slot:** a second push while the receiver held the pending handoff
  returned HTTP 503 — sender state `busy`, detail "SamuelClient is busy
  with another handoff — try again".
- **Cleanup:** the receiver removed `samuel-client.json` on shutdown; the
  sender's stale `mca-client.json` is neutralized by the pid check.
- The sender reported `game_reachable: false` for the unreachable join
  target, as designed.

Not verified in this run (recorded honestly):

- **UI rendering** of the HandoffBanner — the GUI window opened on
  `DISPLAY=:0`, but its content was not inspected (no GUI automation).
- The receiver-side **"server unreachable" degradation on join** — the
  `LaunchGame` preflight is covered by the binding test
  `TestLaunchGameJoinTargetPreflight`; nobody clicked Join during the e2e
  run, so the click path stayed unexercised.
- Deep-link wake from a **never-started** receiver: self-registration
  happens on first startup, and the receiver had run once before Flow B.
