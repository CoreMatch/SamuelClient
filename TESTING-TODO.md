# Testing TODO & Open Decisions

Created 2026-11-05 alongside the first test suite (see commits on branch
`test/add-test-suite`). Items below were found while writing the tests and
need an owner decision before being acted on.

## Resolved in the second round (branch `fix/test-hardening`)

- **#2 (orphan `<id>.json`)** — fixed: `InstallVersion` now removes the
  version directory it created when any later stage fails. A pre-existing
  directory (re-install over a working copy) is deliberately kept so a
  transient download error cannot destroy a playable install; shared
  `libraries/` are untouched. Covered by the "no residue" assertions and
  `TestInstallVersionFailureKeepsPreExistingInstall`.
- **#3 (silent success for unknown ids)** — fixed: `JavaRepository.Delete`
  and `SetDefault` check `RowsAffected` and return a not-found error wrapping
  `sql.ErrNoRows`. This also stops `SetDefault` from clearing every default
  flag when the id does not exist. Covered at repository and binding level.

## Decisions needed

1. **LaunchGame success path is untested.** It spawns a real detached game
   process, which unit tests must not do. Decision: add an integration test
   that uses a fake `java` (shell script) plus a fixture version JSON, or
   leave it to manual verification?
4. **`database.Transaction` has no production consumer.** It is now covered by
   tests, but nothing calls it. Decision: delete, or keep as infrastructure?
5. **Wails event emission is not unit-tested.** `launcher:log` /
   `download:progress` emission needs a real Wails runtime context
   (`runtime.EventsEmit`), which unit tests cannot create. The emit call sites
   are exercised (with nil context they no-op), but delivery to the frontend
   is only verified by reading the code. Decision: accept, or add a `wails
   dev`-based smoke test?
6. **`GetJavaPath` fallback branches are environment-dependent.** Only the
   JAVA_HOME branch is tested deterministically; the common-paths/`"java"`
   fallback depends on the machine. Decision: accept partial coverage?
7. **Frontend has no automated tests** (no test runner configured in
   `package.json`). This round only guarantees `tsc` + `vite build` (plus a
   working `npm run lint`). Decision: introduce Vitest (or similar) for the
   React views?
8. **Triage of the unwired service methods (2026-11-05, branch
   `fix/round3-triage`).** Each method was grep'd for callers and classified:
   - `DownloadAssets` — **WIRED** into `InstallVersion` as stage 5. Reason:
     `BuildLaunchCommand` passes `--assetsDir <gameDir>/assets`, but nothing
     ever populated it, so every fresh install launched without textures or
     sounds — a user-visible feature gap, one wiring step away. The
     `download:progress` event gains the `"asset"` stage (contract updated in
     `bindings.go` and README). Tests: happy path now asserts the asset
     requests/files/progress stage; `TestInstallVersionAssetFailureCleansUp`
     asserts no residue on asset-stage failure.
   - `VerifyMD5` — **DELETED** (with its test). No caller; the ecosystem
     checksums in version/library manifests are SHA-1, so an MD5 helper fits
     no current or planned verification path.
   - `Unzip` — **DELETED** (with its test). Only useful for Forge installer
     extraction, which is not implemented (see below); kept dead code only
     inflates coverage.
   - `RefreshAuthlibToken` — **DELETED**. Thin wrapper with no caller; the
     binding calls `RefreshAuthlibTokenWithProfile` directly (the no-profile
     case is tested by passing `""`).
   - `GetFabricInstaller` / `DownloadForgeInstaller` — **NEED AUTHOR
     DECISION**. These only download installer jars. The UI offers a
     Fabric/Forge loader dropdown and `LaunchGameRequest` carries
     loader_type/loader_version, but nothing installs the loader (no
     installer execution, no loader version JSON generation), so modded
     launch is a feature gap bigger than one wiring step: it needs a design
     (run installer via detected Java, or merge loader profiles into the
     version JSON). Options: (a) design and implement loader installation,
     (b) hide the loader dropdown until then, (c) delete the two downloaders.
9. **`UserRepository`, `TaskRepository`, `FileRepository` retired
   (2026-11-05, branch `fix/round3-triage`).** Zero production callers were
   found (grep across `*.go`), so the repositories, their models
   (`User`/`Task`/`File`, plus the never-referenced `Mod`/`ModLoader`/
   `DownloadTask` models and related constants) and their tests were deleted.
   Handled exactly like the `logs` table: fresh databases no longer create
   `users`/`tasks`/`files`; no DROP migration is issued, so existing
   deployments keep the old tables inert on disk. Regression test:
   `TestMigrationsDoNotCreateRetiredTables`.
10. **Frontend lint warnings fixed (2026-11-05, branch `fix/round3-triage`).**
    `npm run lint` now reports 0 errors / 0 warnings without any
    `eslint-disable`: the `any` usages got real interfaces
    (`BackendInstalledVersion`, `FabricLoaderEntry`, `ForgeVersionEntry`,
    shared `InstalledVersion` in `src/types.ts`), the `useEffect` dependency
    was added, and `saveAccount`/`StoredAccount` moved to
    `src/accountStorage.ts` so component files only export components
    (react-refresh).

## Known behavior documented by tests (no action needed unless disagreed)

- `DownloadLibraries` continues after a failed library and reports the last
  error; `DownloadNatives` never fails (skips broken entries).
- A second `database.Initialize` in the same process is a no-op (`sync.Once`).
- The retired `logs`, `users`, `tasks` and `files` tables are not created on
  fresh databases; existing tables in already-deployed databases stay inert
  (ADR-0004 follow-ups).
- A failed `InstallVersion` removes a freshly created version directory but
  never touches a pre-existing one; `JavaRepository.Delete`/`SetDefault`
  fail with a wrapped `sql.ErrNoRows` for unknown ids.
