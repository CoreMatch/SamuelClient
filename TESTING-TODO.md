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
8. **Tested but unwired service methods.** `DownloadAssets`,
   `GetFabricInstaller`, `DownloadForgeInstaller`, `Unzip`, `VerifyMD5`, and
   the thin `RefreshAuthlibToken` wrapper have no production caller (they are
   building blocks for flows the bindings do not expose yet; note that a full
   launch eventually needs asset downloads). Kept this round because they are
   tested and on the documented launcher path. Decision: wire them into
   bindings/`InstallVersion`, or delete?
9. **`UserRepository`, `TaskRepository`, `FileRepository` (and the `users`,
   `tasks`, `files` tables) have no production caller** — only tests use them.
   Kept this round as a conservative choice. Decision: retire them like the
   `logs` table (ADR-0004 follow-up), or keep for planned features?
10. **Seven pre-existing frontend lint warnings remain** (four
    `no-explicit-any`, one `react-hooks/exhaustive-deps`, one
    `react-refresh/only-export-components`, plus related). Fixing them means
    typing Wails response payloads and touching hook dependencies. Decision:
    fix, or accept as warnings?

## Known behavior documented by tests (no action needed unless disagreed)

- `DownloadLibraries` continues after a failed library and reports the last
  error; `DownloadNatives` never fails (skips broken entries).
- A second `database.Initialize` in the same process is a no-op (`sync.Once`).
- The retired `logs` table is not created on fresh databases; existing tables
  in already-deployed databases stay inert (ADR-0004 follow-up).
- A failed `InstallVersion` removes a freshly created version directory but
  never touches a pre-existing one; `JavaRepository.Delete`/`SetDefault`
  fail with a wrapped `sql.ErrNoRows` for unknown ids.
