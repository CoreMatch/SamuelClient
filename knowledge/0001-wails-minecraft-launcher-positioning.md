# ADR-0001: Wails-based Cross-Platform Minecraft Launcher

- **Status**: accepted
- **Date**: 2026-11-04

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

## Evidence

- Tech stack: `go.mod:1,3,6-8` (module `HRPMLW`, Gin, Wails v2.12.0, sqlite);
  `wails.json:3-4` ("Samuel Client"); `frontend/package.json:13-22,38-39`
  (React 19 + MUI + Vite), package name `hrpml-ml` at `:2`.
- Window title and startup: `main.go:19-20`; `app.go:35-45` (startup init
  SQLite + Gin), `app.go:55-59` (Gin on `config.ServerPort`); default port
  `internal/config/config.go:17` (`34501`).
- Routes: `internal/routes/routes.go:37-74` (route setup), `:135-144` (Java
  CRUD), `:149-194` (MC launcher group).
- Launch flow: `frontend/src/views/HomeView.tsx:262-294` (`handleLaunch`),
  `internal/handlers/mc_handlers.go:441-517` (launch handler),
  `internal/services/mc_services.go:1166-1272` (`BuildLaunchCommand`),
  `:1285-1356` (`LaunchGame`).
- Downloads/mirror: `internal/services/mc_services.go:368-374` (BMCLAPI base
  URL), `:489-497` (URL rewrite), `:465` (version manifest), `:670,722,744`
  (fabric/forge).
- Java management: `internal/handlers/db_handlers.go:561-641`; auto-detect
  `internal/services/mc_services.go:968-990`.
- README positioning: `README.md:3` ("cross-platform Minecraft launcher built
  with Wails").
- Commits: `4284748` (full launcher functionality), `23bf03a` (Java management),
  `da69467` (auto-detected system Java), `f87dea6` (launcher config
  persistence), `19fd6c3` (rename to Samuel Client + authlib).
- Not verified: none material to the positioning claim.
