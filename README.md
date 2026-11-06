# Samuel Client

A cross-platform Minecraft launcher built with [Wails](https://wails.io/) (Go + React/TypeScript), supporting vanilla, Fabric, and Forge profiles with offline and authlib-injector (Yggdrasil) account authentication.

## Features

- **Account management** — offline and third-party (authlib-injector / Yggdrasil) login flows, with token refresh and validation
- **Version management** — browse and download vanilla Minecraft versions, plus Fabric and Forge mod loaders
- **Mod management** — open the `mods` folder for the active profile directly from the UI
- **Java management** — track installed Java runtimes and set a default per profile
- **Persistent storage** — embedded [SQLite](https://www.sqlite.org/) database for settings and Java installations
- **Cross-platform** — Windows, macOS, and Linux builds via Wails

## Tech stack

| Layer    | Technology |
|----------|------------|
| Backend  | Go 1.25, [Wails v2](https://wails.io/), [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) |
| Frontend | React 19, TypeScript 5, [Vite 6](https://vitejs.dev/), [Material UI v9](https://mui.com/) |
| Build    | Wails CLI (WebKitGTK 2.41+ on Linux) |

## Project layout

```
SamuelClient/
├── app.go                  # Wails App struct — entry point for Go ↔ JS bindings
├── main.go                 # Wails application bootstrap
├── wails.json              # Wails project configuration
├── go.mod / go.sum         # Go module definition
├── internal/
│   ├── database/           # SQLite initialization and access
│   ├── models/             # Domain & API models
│   ├── repository/         # Data access layer
│   └── services/           # Auth, download, and launcher services
├── frontend/
│   ├── src/
│   │   ├── components/     # React views (Home, Versions, Accounts, Mods, Settings, Login)
│   │   ├── App.tsx         # Main app shell with navigation drawer
│   │   └── main.tsx        # React entry point
│   ├── index.html
│   ├── package.json
│   ├── tsconfig.json
│   └── vite.config.ts
├── build/                  # Platform-specific build assets (icons, manifests, installer)
└── docs/                   # Reference docs (launcher standard, BMCLAPI, authlib-injector)
```

## Prerequisites

- **Go** 1.25 or newer
- **Node.js** 18+ and **npm**
- **Wails CLI** v2: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Platform build dependencies — see the [Wails installation guide](https://wails.io/docs/gettingstarted/installation):
  - **Linux**: GCC, libgtk-3-dev, libwebkit2gtk-4.0-dev (or 4.1), pkg-config
  - **macOS**: Xcode Command Line Tools
  - **Windows**: MinGW-w64 / MSYS2, WebView2 runtime

Verify your environment with `wails doctor`.

## Getting started

Clone the repository and run the app in live development mode (with hot reload for the frontend):

```bash
git clone https://github.com/CoreMatch/SamuelClient.git
cd SamuelClient
wails dev
```

`wails dev` starts a Vite dev server and reloads the UI on change. Go method changes trigger an automatic rebuild. A browser devtools server is also exposed on `http://localhost:34115` for inspecting the Go bindings from the browser console.

## Building

To produce a redistributable, production-mode binary for your current platform:

```bash
wails build
```

The output binary is written to `build/bin/` (`SamuelClient` / `SamuelClient.exe`). Cross-compilation is supported via `wails build -platform windows/amd64` etc. — see the [Wails CLI reference](https://wails.io/docs/reference/cli#build).

## Backend interface

The frontend talks to the backend exclusively through Wails bindings (there is
no frontend-facing HTTP listener). All bound methods return one uniform response shape:
`{ success, data, error }`.

| Area | Bindings |
|------|----------|
| Auth (read) | `GetAuthlibMeta` |
| Versions (read) | `GetVersions`, `GetFabricVersions`, `GetForgeVersions`, `GetInstalledVersions` |
| Java (read) | `GetJavaInstallations` |
| Config (read) | `GetLauncherConfig` |
| Match handoff (read) | `GetPendingHandoff`, `GetInteropStatus` |
| Auth (write) | `AuthlibLogin`, `AuthlibRefresh`, `AuthlibValidate` |
| Install / launch | `DownloadVersion`, `InstallFabricLoader`, `InstallForgeLoader`, `LaunchGame` |
| Config (write) | `UpdateLauncherConfig` |
| Java (write) | `AddJavaInstallation`, `UpdateJavaInstallation`, `DeleteJavaInstallation`, `SetDefaultJavaInstallation` |
| Match handoff (write) | `DismissHandoff` |

Live data is pushed from Go to the frontend via Wails runtime events:

- `launcher:log` — one event per output line of the launched game process or
  of the Forge installer while `InstallForgeLoader` runs.
  Payload: `{ "source": "stdout" | "stderr", "line": string }`.
- `download:progress` — progress while `DownloadVersion` installs a version
  or `InstallFabricLoader`/`InstallForgeLoader` install a mod loader.
  Payload: `{ "version_id": string, "stage": "jar" | "library" | "native" | "asset" | "loader", "name": string, "downloaded": number, "total": number }`.
- `interop:handoff` — match handoff lifecycle from the matchmaking client.
  Payload: `{ "status": "pending" | "dismissed" | "gone" | "invalid" | "version_mismatch" | "pull_timeout", "handoff": <payload when pending>, "message": <readable text for degraded statuses> }`.

## Match handoff (MatchCore interop)

SamuelClient is the receiver of the MatchCore match handoff protocol
(`INTEROP-SPEC.md`, protocol v1): after matchmaking succeeds, the
matchmaking client hands the match over and SamuelClient launches the game
straight into the match server using its own logged-in account.

- Loopback-only HTTP listener on `127.0.0.1` with a per-process random
  token, published in a 0600 discovery file under
  `$XDG_RUNTIME_DIR/matchcore-interop/samuel-client.json` (pid-checked by
  readers; removed on clean exit).
- The `matchcore://join?...` deep link is an invocation hint only (handoff
  id + protocol version, never credentials). On Linux the launcher
  self-registers an `x-scheme-handler/matchcore` .desktop entry at startup;
  Windows/macOS registration is a documented gap.
- If the launcher was woken by a deep link, it pulls the match parameters
  from the matchmaking client's loopback endpoint (30 s retry window) and
  then behaves like a pushed handoff.
- Joining requires a logged-in account; when none is available the UI
  guides login and auto-resumes the handoff afterwards. Unreachable match
  servers, missing game versions, protocol mismatches and expired handoffs
  all degrade with readable messages — the matchmaking result is never
  dropped silently.
- Match results are still reported by the BedwarsRel plugin; the launcher
  only gets the player into the server.

## Frontend

The UI is a single-page React app written in TypeScript and built with Vite. Material UI provides the component library. Views are organized around a left navigation drawer:

- **Play** — launch the currently selected version with the active account
- **Versions** — browse, download, and select Minecraft versions
- **Accounts** — add and switch between offline and Yggdrasil accounts
- **Mods** — open the active profile's `mods` folder
- **Settings** — application preferences

Run frontend-only checks:

```bash
cd frontend
npm run lint
npm run typecheck
```

Both need the generated Wails bindings (`frontend/wailsjs`, not committed).
Run `wails generate module` once from the repository root to create them;
`wails dev` and `wails build` regenerate them automatically.

## License

This project is licensed under the **GNU Affero General Public License v3.0** — see the [LICENSE](./LICENSE) file for the full text.

## Acknowledgements

- [Wails](https://wails.io/) — Go + web frontend framework
- [Material UI](https://mui.com/) — React component library
- [authlib-injector](https://github.com/yushijinhun/authlib-injector) — Yggdrasil authentication reference
