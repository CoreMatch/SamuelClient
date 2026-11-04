# ADR-0001: Wails-based Cross-Platform Minecraft Launcher

- **Status**: accepted
- **Date**: 2026-11-04

> **Supersession note (2026-11-05):** the positioning in this entry still
> stands, but its HTTP-boundary details do not: the Gin server, the
> `/api/v1/...` routes, `SERVER_PORT`, and the frontend→Gin HTTP calls were
> all retired by [ADR-0004](0004-local-http-boundary.md). Every backend call
> now goes through a Wails binding, and the process opens no listening port.
> Evidence lines below citing `internal/routes`, `internal/config`, or
> `frontend/src/variables.ts` refer to files that no longer exist; they are
> kept as the historical record (time-point anchored).

## Context

The project needed a desktop Minecraft launcher that manages game versions,
Java runtimes, and account login, with a modern web-style UI but a native
window. SamuelClient (Samuel Client) is that launcher.

## Decision

SamuelClient is a **cross-platform Minecraft launcher built with Wails**:

- Backend is Go (module `HRPMLW`), using Gin for a local HTTP API and SQLite
  for local persistence; the Wails window hosts a React frontend.
- A local Gin server listens on a configurable port (default `34501`) and
  exposes `/api/v1/...` routes for auth, versions, mod loaders (Fabric/Forge),
  downloads, Java management, and launching.
- Launch flow: the frontend calls `POST /api/v1/launcher/launch`; the backend
  builds the Java command line from the version JSON, classpath, natives, and
  game arguments, then spawns a detached process and captures logs/PID.
- Version downloads go through the BMCLAPI mirror with URL rewriting; Java
  installations are managed in SQLite with auto-detection of system Java.

## Consequences

- The launcher is self-contained: it does not require an external launcher
  service; all state is local SQLite plus the configured auth server.
- The frontend talks to the local Gin server over HTTP rather than only Wails
  bindings; some auth-related local endpoints have backend implementations but
  no frontend caller (see entry 0002).
- Cost: module/package names (`HRPMLW`, `hrpml-ml`) were never renamed to match
  the "Samuel Client" branding; only `wails.json` and the window title changed.
- CMCL residue: `cmcl.jar` (an external Minecraft CLI launcher) is a leftover
  from the frontend rewrite; it was **never committed to version control**, so
  there is no file to remove. Its only in-repo reference was the dead
  `CMCL_CONFIG` block in `frontend/src/variables.ts` (zero consumers anywhere
  in the repo); that file was deleted on 2026-11-04 during the ADR-0004
  dead-code cleanup, so no CMCL reference remains.

## Evidence

- Tech stack: `go.mod:1,3,6-7` (module `HRPMLW`, Wails v2.12.0, sqlite);
  `wails.json:3-4` ("Samuel Client");
  `frontend/package.json (field: "dependencies")` (React 19 + MUI) and
  `frontend/package.json (field: "vite")` (Vite), package name `hrpml-ml` at
  `:2`.
- Window title and startup: `main.go:19-20`; `app.go:38-45` (startup init
  SQLite), `app.go:L55-L59 @ 27745e6` (Gin on `config.ServerPort`); default
  port `internal/config/config.go:L17 @ 27745e6` (`34501`).
- Routes: `internal/routes/routes.go:L37-L74 @ 27745e6` (route setup),
  `internal/routes/routes.go:L135-L144 @ 27745e6` (Java CRUD),
  `internal/routes/routes.go:L149-L194 @ 27745e6` (MC launcher group).
- Launch flow: `frontend/src/components/HomeView.tsx::handleLaunch`,
  `bindings.go::LaunchGame` (launch binding),
  `internal/services/mc_services.go::BuildLaunchCommand`,
  `internal/services/mc_services.go::LaunchGame`.
- Downloads/mirror: `internal/services/mc_services.go::NewBMCLAPIService`
  (BMCLAPI base URL), `internal/services/mc_services.go::toBMCLAPI` (URL
  rewrite), `internal/services/mc_services.go::GetVersionList` (version
  manifest), `internal/services/mc_services.go::GetFabricVersions`,
  `internal/services/mc_services.go::GetForgeVersions`,
  `internal/services/mc_services.go::DownloadForgeInstaller` (fabric/forge).
- Java management: `bindings.go::GetJavaInstallations`,
  `bindings.go::AddJavaInstallation`, `bindings.go::UpdateJavaInstallation`,
  `bindings.go::DeleteJavaInstallation`, `bindings.go::SetDefaultJavaInstallation`;
  auto-detect `internal/services/java.go:54-103`.
- README positioning: `README.md:3` ("cross-platform Minecraft launcher built
  with Wails").
- Commits: `4284748` (full launcher functionality), `23bf03a` (Java management),
  `da69467` (auto-detected system Java), `f87dea6` (launcher config
  persistence), `19fd6c3` (rename to Samuel Client + authlib).
- CMCL residue check (2026-11-04): `git log --all --full-history -- "*cmcl*"`
  shows no `cmcl.jar` was ever committed; the only cmcl-named file ever tracked
  was `frontend/docs/CMCL_Commands_Report.md` (added in `349774e`, moved to
  `docs/` in `756cbf0`, removed in `7095d3b`). The last reference was
  `frontend/src/variables.ts:L12-L15 @ 27745e6`
  (`CMCL_CONFIG`, `cmclPath` default `java -jar ./cmcl.jar`); a repo-wide
  search found no consumer of `CMCL_CONFIG`/`cmclPath` in code, build scripts,
  README, or CI, and `variables.ts` was deleted during the ADR-0004 cleanup.
- Not verified: none material to the positioning claim.
