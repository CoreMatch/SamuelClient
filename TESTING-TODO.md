# Testing TODO & Open Decisions

Created 2026-11-05 alongside the first test suite (see commits on branch
`test/add-test-suite`). Items below were found while writing the tests and
need an owner decision before being acted on.

## Decisions needed

1. **LaunchGame success path is untested.** It spawns a real detached game
   process, which unit tests must not do. Decision: add an integration test
   that uses a fake `java` (shell script) plus a fixture version JSON, or
   leave it to manual verification?
2. **InstallVersion leaves an orphan `<id>.json` when a later stage fails.**
   Stage 1 persists the version JSON before the jar download; if the jar or a
   library fails, the JSON stays on disk (the version is still not listed as
   installed, because the UI requires json+jar). Decision: delete the version
   directory on failure, or keep the current behavior?
3. **`JavaRepository.Delete`/`SetDefault` silently succeed for unknown ids**
   (SQLite DELETE/UPDATE of zero rows is not an error; no RowsAffected check).
   Decision: add existence checks and return not-found errors?
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
   `package.json`). This round only guarantees `tsc` + `vite build`.
   Decision: introduce Vitest (or similar) for the React views?

## Known behavior documented by tests (no action needed unless disagreed)

- `DownloadLibraries` continues after a failed library and reports the last
  error; `DownloadNatives` never fails (skips broken entries).
- A second `database.Initialize` in the same process is a no-op (`sync.Once`).
- The retired `logs` table is not created on fresh databases; existing tables
  in already-deployed databases stay inert (ADR-0004 follow-up).
