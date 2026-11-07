# ADR-0005: Frontend Test Framework — Vitest + React Testing Library

- **Status**: accepted

## Context

The frontend (Vite + React 19 + TypeScript) gained component tests during the
loader-install work, and the framework choice was asked to be re-evaluated
against the alternatives: **Vitest**, **Jest**, and **@testing-library/react**
(RTL). One clarification up front: these are not three interchangeable
alternatives — Vitest and Jest are *test runners*, while RTL is a *component
testing library* that runs under either. The real runner decision is
Vitest vs Jest; RTL is the interaction layer in both worlds.

Current state at evaluation time: Vitest is already wired in
(`frontend/vite.config.ts` `test` block, `npm test`), with jsdom,
`globals: true` (so RTL registers its automatic DOM cleanup), a matchMedia
stub for MUI (`src/vitest.setup.ts`), and 10 tests in 2 files
(`HomeView.test.tsx`, `LoginView.test.tsx`) that mock the generated Wails
bindings via `vi.mock('../../wailsjs/go/main/App')`.

## Comparison

| Dimension | Vitest | Jest | @testing-library/react |
|---|---|---|---|
| Role | Test runner, Vite-native | Test runner, bundler-agnostic | Component query/interaction library (not a runner) |
| Integration cost in this repo | Already integrated: reuses `vite.config.ts`, native TS/ESM/JSX transform, one config for build and test | Needs a parallel transform pipeline (`babel-jest` or `ts-jest`), jsdom env config, `moduleNameMapper` for CSS and the `wailsjs` imports — a second config that can drift from Vite's | n/a — already in use under Vitest in both test files |
| Migration cost | 0 — the 10 existing tests are Vitest tests | Rewrite `vitest` imports, `vi.mock`/`vi.fn` → jest equivalents (API-similar but not drop-in for module factories), re-create the setup file semantics, then maintain two configs | None either way |
| Compatibility with the existing 10 tests | Native (they were written for it) | Indirect; expect fixups around mocks and jsdom wiring | Full — tests already use RTL queries/fireEvent |
| Watch/dev loop | Shares Vite's transform cache, fast | Slower cold start here (separate transform) | n/a |
| Risk | None observed | Config drift between build (Vite) and test (Jest) is the classic failure mode | n/a |

## Decision

Keep the status quo: **Vitest + jsdom + @testing-library/react**. Do not
introduce Jest — it would duplicate the build tooling configuration for zero
capability gain, and every existing test would need migration rather than
keep working. RTL stays as the component interaction layer regardless of
runner. No `package.json` change was made for this evaluation.

## Consequences

- One toolchain for build and test; `npm test` is also what CI runs.
- Wails bindings stay mocked at the module level (`vi.mock`), which is the
  seam that keeps tests hermetic; this works identically under Vitest's
  module system.
- If a future need is truly Jest-specific (e.g. a snapshot-heavy design
  system tooling), revisit; nothing in the current roadmap requires it.
- Remaining coverage gap is unchanged and tracked in TESTING-TODO #7:
  VersionManager/AccountManager/SettingsView/ModManager have no tests yet.

## Evidence

- `frontend/package.json` — devDependencies: `vitest ^5.1.0`,
  `@testing-library/react ^16.3.3`, `@testing-library/dom ^10.4.2`,
  `jsdom ^30.1.3`; scripts: `"test": "vitest run"`.
- `frontend/vite.config.ts:L9-L16 fp=1f5beb9e ~ plugins: [tailwindcss(), react()],` — the `test` block (jsdom, globals, setup
  file, include pattern).
- `frontend/src/vitest.setup.ts` — matchMedia stub for MUI under jsdom.
- Patrol run on branch `chore/nightly-patrol`: `npm test` → 2 test files,
  10 tests passed; `npm run lint` → 0 errors / 0 warnings.
- Mock seam: `frontend/src/components/LoginView.test.tsx:L8-L11 fp=93f13eb1 ~ vi.mock('../../wailsjs/go/main/App', () => ({` and
  `HomeView.test.tsx:18` (`vi.mock('../../wailsjs/go/main/App')`).
