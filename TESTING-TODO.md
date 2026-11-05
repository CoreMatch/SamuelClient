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
   Escalated to `PENDING-DECISIONS.md` #9 by the 2026-11-06 patrol.
7. **Frontend test framework introduced (2026-11-05, branch
   `feat/loader-install`).** Vitest + jsdom + @testing-library/react, wired
   through `vite.config.ts` (`npm test`, also added to CI). Key-interaction
   coverage: LoginView (authlib validation/success/failure, offline login
   via the type select) and HomeView (Play gating, launch request payload,
   backend error surfacing), all Wails bindings mocked. Remaining gap: no
   tests for VersionManager/AccountManager/SettingsView/ModManager yet.
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
     **Real-network verified (2026-11-05, branches `chore/loader-ui-and-verify`
     and `feat/loader-launch-complete`)**:
     Fabric 0.20.1 for MC 1.20.4 via the live BMCLAPI mirror — profile JSON
     on disk, 8 loader libraries downloaded and content-validated (zip
     magic), `launcher_profiles.json` entry written, `GetInstalledVersions`
     recognizes it, and `BuildLaunchCommand` emits the fabric launch command
     (KnotClient mainClass, `-DFabricMcEmu` from the profile's `arguments`,
     parent jar on the classpath). Forge 1.20.4-49.0.3 — real installer ran
     on JDK 25 (~40s), produced `versions/1.20.4-forge49.0.3/`, injected its
     `launcher_profiles.json` entry (kept untouched by the service), removed
     the installer jar, and the launch command built from the real forge
     version JSON carries `--launchTarget forge_client` plus the parent jar.
     The live runs exposed and fixed real bugs: fabric meta libraries without
     file URLs (resolved from maven coordinates / repository bases via the
     mirror), mirror CDNs 403-ing Go's default User-Agent (downloads now
     send a launcher UA), the forge installer download URL (maven layout),
     the missing `launcher_profiles.json` requirement (seeded), and the
     missing `arguments` expansion in `BuildLaunchCommand` (loader versions
     now expand Mojang-style game/jvm arguments with rules and ${variable}
     substitution; vanilla keeps the legacy construction).
     UI follow-up (done 2026-11-05, `chore/loader-ui-and-verify`): the
     HomeView loader dropdown now calls `InstallFabricLoader`/
     `InstallForgeLoader` and refreshes the version list on success.
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
      schedule the cancellation work. Escalated to `PENDING-DECISIONS.md` #10
      by the 2026-11-06 patrol.

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

## Nightly patrol results (branch `chore/nightly-patrol`)

**Triage of #4/#5/#7** — all three were already closed before this patrol;
re-verified against the current code:

- #4 (`database.Transaction`): deletion complete, zero references remain
  (grep across `*.go`).
- #5 (event emission not unit-tested): acceptance stands; emit call sites
  remain nil-context safe, contract documented in `bindings.go` and README.
- #7 (frontend framework): Vitest is live — `npm test` passes 10/10 tests
  in 2 files. The framework re-evaluation (vitest vs jest vs RTL) is written
  up in `knowledge/0005-frontend-test-framework-evaluation.md`: keep
  Vitest + jsdom + @testing-library/react; Jest rejected (duplicate
  transform config, migration cost, zero gain). No `package.json` change.

**Quality patrol** (all commands run in the patrol worktree):

| Check | Result |
|---|---|
| `npm run lint` | 0 errors / 0 warnings |
| `npm test` | 2 files, 10 tests passed |
| `npm run build` (tsc + vite) | OK |
| `go build ./...` | OK |
| `go vet ./...` | OK |
| `go test -race ./...` | all packages pass |
| `gofmt -l` | 4 files unformatted (alignment only) — fixed on this branch |
| `go mod tidy` | no diff (go.mod/go.sum unchanged) |
| `npm ls --all` | no extraneous / invalid / missing packages |
| Coverage (`go test -cover`) | main 75.8%, database 70.4%, repository 80.4%, services 81.5% |
| Bundle (`vite build`) | `index.js` 581.73 kB (gzip 176.77 kB), CSS 0.34 kB, HTML 0.46 kB — only 3 artifacts, single JS chunk trips Vite's >500 kB warning → `PENDING-DECISIONS.md` #12 |

**Binding surface self-check** — `bindings.go` has 19 methods; the generated
`wailsjs/go/main/App.d.ts` has the same 19 (one-to-one); every frontend
import resolves (tsc passes). Diff against frontend call sites:

| Binding | Frontend caller |
|---|---|
| `GetAuthlibMeta`, `AuthlibLogin` | LoginView |
| `GetVersions`, `DownloadVersion` | VersionManager |
| `GetLauncherConfig`, `GetJavaInstallations`, `UpdateLauncherConfig`, `AddJavaInstallation`, `UpdateJavaInstallation`, `DeleteJavaInstallation`, `SetDefaultJavaInstallation` | SettingsView (`GetJavaInstallations` also HomeView) |
| `GetInstalledVersions`, `GetFabricVersions`, `GetForgeVersions`, `InstallFabricLoader`, `InstallForgeLoader`, `LaunchGame` | HomeView |
| `AuthlibRefresh` | **none** |
| `AuthlibValidate` | **none** |

No frontend call targets an unbound method. The two unused bindings are
escalated as `PENDING-DECISIONS.md` #11 (wire into account restore vs
delete per the no-dead-code precedent).

**Event consumer field defense** — both consumers tolerate missing fields:

- `launcher:log` (HomeView.tsx `EventsOn` handler): null-payload guard,
  `source || 'stdout'`, `line || ''` — a payload missing either field
  renders as an empty stdout line, never throws.
- `download:progress` (VersionManager.tsx `EventsOn` handler): null guard,
  drops events without `version_id`, `stage || ''`, and computes the
  percentage only when `total > 0` and `typeof downloaded === 'number'` —
  missing numeric fields render a bare stage label, never divide by zero.

**Escalations** (appended to `/home/lnb/SamuelPortal-docs/PENDING-DECISIONS.md`):
#9 (GetJavaPath partial coverage, from item 6 above), #10 (cancellation
plumbing, from item 11 above), #11 (unused auth bindings), #12 (bundle
splitting).
