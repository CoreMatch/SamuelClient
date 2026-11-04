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

1. **LaunchGame success path — covered by an end-to-end smoke test
   (2026-11-05, branch `fix/round3-triage`).** `smoke_test.go`
   (`TestSmokeLoginInstallLaunch`) drives the real bindings through
   AuthlibLogin (fake Yggdrasil httptest server) → DownloadVersion (fake
   BMCLAPI httptest server) → LaunchGame (shell-script "java"), then polls
   the launch log for the process's stdout/stderr lines and clean exit. No
   real network, accounts, or GUI. Remaining gap: it exercises a fake java,
   not a real Minecraft client — real-game verification stays manual.
4. **`database.Transaction` deleted (2026-11-05, branch
   `feat/loader-install`).** It had no production consumer (grep-verified;
   only its own test called it), so per the standing "do not keep dead
   code" policy the helper and `TestTransactionCommitAndRollback` were
   removed. `JavaRepository.SetDefault` keeps its own explicit transaction,
   which is the only transactional code path.
5. **Wails event emission: NOT unit-tested — accepted.**
   `launcher:log` / `download:progress` emission needs a real Wails runtime
   context (`runtime.EventsEmit`), which unit tests cannot create.
   Mitigations in place: the emit call sites are exercised with a nil
   context (no-op path), the payload contract is documented in `bindings.go`
   and README, the smoke test verifies the underlying log/download paths,
   and delivery was manually confirmed on a `wails build` artifact.
   Reopening only makes sense if the frontend gains an integration harness.
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
   - `GetFabricInstaller` / `DownloadForgeInstaller` — **WIRED**
     (2026-11-05, branch `feat/loader-install`): `InstallFabricLoader`
     fetches the Fabric profile JSON, persists it under
     `versions/<profile id>/` with `inheritsFrom` (resolved by
     `BuildLaunchCommand` at launch) and downloads the loader libraries;
     `InstallForgeLoader` downloads the installer jar, runs
     `java -jar <installer> --installClient` headless with a 10-minute
     timeout, streams its output as `launcher:log` events, verifies the
     generated version directory and removes the transient installer jar.
     Both emit `download:progress` with stage `"loader"`, clean up fresh
     directories on failure, and are retryable.
     **Not verified**: against a real Fabric/Forge installer and real JRE
     (tests use httptest meta + a shell-script installer covering exit 0 /
     non-zero / timeout). Known limitation: the stock Forge installer
     targets the platform-default Minecraft directory; `--installClient`
     without a directory flag cannot be redirected on older installers, so
     `gameDir` should be the default one. Decision left for the author:
     whether to add modern-installer `--installDir` handling.
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
11. **Process lifecycle: no cancellation plumbing (author confirmation
    wanted).** Recorded 2026-11-05 (branch `feat/loader-install`).
    - Impact: an in-flight `DownloadVersion` or Forge installer run cannot
      be cancelled from the UI — the Forge run is bounded only by its
      10-minute timeout; a user "retry" can run two installs concurrently;
      the launched game is detached on purpose (survives the launcher) and
      is not meant to be killable from the launcher.
    - Minimal change (NOT implemented — alters the binding surface):
      thread `context.Context` through `DownloadFile`/`InstallVersion`/
      `InstallForgeLoader` (requests via `http.NewRequestWithContext`,
      processes via `exec.CommandContext`/`cmd.Cancel`), keep per-version
      cancel funcs in `App`, add a `CancelInstall(versionID)` binding and a
      cancel button in the UI.
    - Decision needed: accept the status quo (timeouts + retry tolerance) or
      schedule the cancellation work.

## Known behavior documented by tests (no action needed unless disagreed)

- A failed `DownloadFile` (truncated stream) removes the partial file so a
  retry is not blocked by "already exists" skips. Limitation: there is no
  cancellation/context plumbing — a hard process kill mid-download can still
  leave partial files behind; adding `context.Context` to the download path
  would be a deliberate follow-up if cancellation is ever required.
- `DownloadLibraries` continues after a failed library and reports the last
  error; `DownloadNatives` never fails (skips broken entries).
- A second `database.Initialize` in the same process is a no-op (`sync.Once`).
- The retired `logs`, `users`, `tasks` and `files` tables are not created on
  fresh databases; existing tables in already-deployed databases stay inert
  (ADR-0004 follow-ups).
- A failed `InstallVersion` removes a freshly created version directory but
  never touches a pre-existing one; `JavaRepository.Delete`/`SetDefault`
  fail with a wrapped `sql.ErrNoRows` for unknown ids.
