# ADR-0009: authlib-injector Agent Bundled in the Binary

- **Status**: accepted

## Context

The launch chain has always been able to inject authlib-injector
(`BuildLaunchCommand` emits `-javaagent:<jar>=<HRPAuthBaseURL>` plus the
prefetched-metadata define), but nothing ever provisioned the jar: the
frontend read a localStorage key (`authlib_jar_path`) that no code path
wrote, the settings UI sent `authlib_jar` empty, and the backend had no
download or bundling mechanism. In practice the `-javaagent` flag never
fired, so HRPAuth sessions could not authenticate in-game on online
servers.

Product decisions recorded with the owner (2026-11-15):

- Scope stays **HRPAuth-only** (ADR-0002 stands): no third-party
  Yggdrasil servers, no generic auth-server entry point.
- The agent jar is **bundled at build time**, not downloaded at runtime
  and not user-selected.
- Match handoff remains limited to built-in account types; legacy
  third-party accounts stay inert (no migration change).

## Decision

- The official authlib-injector **1.2.8** (build 56) jar is committed at
  `internal/services/assets/authlib-injector.jar` and embedded into the
  Go binary (`//go:embed`, `internal/services/authlib_jar.go`).
  SHA-256 verified against the official artifact index:
  `9c7f4343e6c82034958ffb48c14a2cb0c85928be7283103ce17da00c6d5a7b10`.
- `-javaagent` needs a real file, so `EnsureAuthlibInjectorJar`
  materializes the embedded bytes to `~/.hrpml/authlib-injector.jar`
  (the launcher data directory, shared with the SQLite database).
  An identical on-disk copy is left untouched; a missing or outdated copy
  is replaced atomically (temp file + rename).
- `LauncherService.LaunchGame` defaults `config.AuthlibJar` to the
  extracted path for HRPAuth accounts. Extraction failure aborts the
  launch with a clear error — without the agent the game could never
  join an online server. An explicitly configured jar
  (`LauncherConfig.AuthlibJar`, request field `authlib_jar`) still wins
  and is used verbatim; the frontend now always sends it empty.
- Updating the agent is a build-time action: fetch the new jar from the
  official artifact index, verify its SHA-256, replace the asset, and
  bump `AuthlibInjectorVersion` (a test cross-checks the constant
  against the jar manifest's `Implementation-Version`).

## Consequences

- Launching needs no network access for the agent and no user setup;
  the injection path is exercised end-to-end by the smoke test.
- The agent only updates with an app release; a server-side protocol
  break requires a launcher update (accepted trade-off of bundling).
- The binary grows by ~342 KB (the embedded jar).
- The first launch (or the first launch after an update) writes the jar
  into the data directory; concurrent launches are safe because the
  replacement is atomic and content-identical.
- The frontend no longer reads `authlib_jar_path`; the key is dead and
  any value a user may have planted there is ignored.

## Evidence

- Bundling + extraction: `internal/services/authlib_jar.go:17`
  (`go:embed`), `:22` (`AuthlibInjectorVersion`), `:28`
  (`EnsureAuthlibInjectorJar`); data directory from
  `internal/database/database.go:24` (`DefaultConfig`, `~/.hrpml`).
- Launch defaulting: `internal/services/mc_services.go:2117-2127`
  (`LaunchGame`); injection site unchanged at
  `internal/services/mc_services.go:1976-1988` (`BuildLaunchCommand`).
- Binding override semantics: `bindings.go:633-635`
  (`LaunchGameRequest.AuthlibJar`).
- Frontend dead-read removal: `frontend/src/components/HomeView.tsx:388`,
  `frontend/src/App.tsx:178` (both now send `authlib_jar: ''`).
- Tests: `internal/services/authlib_jar_test.go` (extraction,
  idempotence, corrupt-copy replacement, manifest/version cross-check),
  `internal/services/mc_services_test.go:1835`
  (`TestLaunchGameBundledAuthlibJar`: default, override, non-HRPAuth),
  `smoke_test.go:225` (end-to-end `-javaagent` assertion).
- Artifact index `https://authlib-injector.yushi.moe/artifact/latest.json`
  (fetched 2026-11-15): version 1.2.8, build 56, download URL
  `/artifact/56/authlib-injector-1.2.8.jar`, SHA-256 as above; the
  committed jar's checksum was verified to match.