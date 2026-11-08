# ADR-0001: Wails-based Cross-Platform Minecraft Launcher

- **Status**: accepted

## Context

The project needed a desktop Minecraft launcher that manages game versions,
Java runtimes, and account login, with a modern web-style UI but a native
window. SamuelClient (Samuel Client) is that launcher.

## Decision

SamuelClient is a **cross-platform Minecraft launcher built with Wails**:

- Backend is Go (module `HRPMLW`), with SQLite for local persistence; the
  Wails window hosts a React frontend. The frontend talks to the backend
  exclusively through Wails bindings; the process opens no listening port
  (see entry 0004).
- Launch flow: the frontend calls the `LaunchGame` binding; the backend
  builds the Java command line from the version JSON, classpath, natives, and
  game arguments, then spawns a detached process whose output streams as
  `launcher:log` events.
- Version downloads go through the BMCLAPI mirror with URL rewriting; Java
  installations are managed in SQLite with auto-detection of system Java.

## Consequences

- The launcher is self-contained: it does not require an external launcher
  service; all state is local SQLite plus the configured auth server.
- Cost: module/package names (`HRPMLW`, `hrpml-ml`) were never renamed to match
  the "Samuel Client" branding; only `wails.json` and the window title changed.
- CMCL residue: `cmcl.jar` (an external Minecraft CLI launcher) was **never
  committed to version control**, and no CMCL reference remains anywhere in
  the repository — there is no file to remove.

## Evidence

- Tech stack: `go.mod (field: module)` (module `HRPMLW`, Wails v2.12.0, sqlite);
  `wails.json:L3-L4 fp=2d956351 ~ "name": "Samuel Client",` ("Samuel Client");
  `frontend/package.json (field: "dependencies")` (React 19 + @ha/ui) and
  `frontend/package.json (field: "vite")` (Vite), package name `hrpml-ml` at
  `:2`.
- Window title and startup: `main.go::main`; `app.go::NewApp` (startup init
  SQLite).
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
  auto-detect `internal/services/java.go::detectSystemJava`.
- README positioning: `README.md:L3-L3 fp=e13f489d ~ A cross-platform Minecraft launcher built with [` ("cross-platform Minecraft launcher built
  with Wails").
- Commits: `4284748` (full launcher functionality), `23bf03a` (Java management),
  `da69467` (auto-detected system Java), `f87dea6` (launcher config
  persistence), `19fd6c3` (rename to Samuel Client + HRPAuth login).
- CMCL residue check: `git log --all --full-history -- "*cmcl*"` shows no
  `cmcl.jar` was ever committed, and a repo-wide search finds no CMCL
  reference in code, build scripts, README, or CI.
- Not verified: none material to the positioning claim.
