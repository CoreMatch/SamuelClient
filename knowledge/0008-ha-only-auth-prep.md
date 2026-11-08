# ADR-0008: HA-only Auth Prep — Offline Entry Removed, Tokens in the OS Keychain, Product Disclaimer

- **Status**: accepted

## Context

The product direction is fixed (see ADR-0007 for the full design): the
launcher accepts only genuine Microsoft login or HA login, and both end on an
HA account. Microsoft login itself is gated on an Azure application passing
Mojang's allowlist review and is **not** implemented here. Three things do
not depend on that gate and land now:

1. The offline login entry contradicts the direction and the Minecraft Usage
   Guidelines' server clause ("Access to your server: Must only be granted to
   users who have a genuine paid-for version of Minecraft"). Competitor
   practice confirms the direction: Prism gates offline accounts behind a
   valid Microsoft account; HMCL has a public (unmerged) PR citing the same
   EULA clause to remove its offline exemption.
2. Session tokens sat in the webview's localStorage in plaintext. Competitor
   incident history (public HMCL issues: token theft via modpack-bundled
   config files, plaintext-token stealer requests, a 15-item security audit)
   shows this is a real attack surface, not a theoretical one.
3. Third-party Minecraft products must carry a "not official" disclaimer
   (Minecraft Usage Guidelines).

## Decision

### Offline login removed

- `frontend/src/components/LoginView.tsx` offers exactly one login path
  (HRPAuth username/password); the auth-type selector, the offline branch and
  the frontend-fabricated offline UUID generator are gone.
- `internal/services/mc_services.go::resolveUserType` no longer emits the
  `legacy` user type; unknown values fall back to the Yggdrasil `mojang`
  value. The Microsoft (`msa`) mapping is kept as existing scaffolding for
  ADR-0007.
- Stored offline accounts are removed by the storage migration (below); their
  usernames are surfaced through a one-shot notice
  (`frontend/src/accountStorage.ts::consumeOfflineRemovedNotice`, rendered by
  `frontend/src/App.tsx`) so nobody is left staring at a missing account.
  Legacy third-party Yggdrasil records stay listed and inert as before.

### Tokens in the OS keychain

- Library: `github.com/zalando/go-keyring` (Windows Credential Manager /
  macOS Keychain / freedesktop Secret Service). Verified live on the author's
  KDE desktop (kwalletd6 Secret Service) via the opt-in test
  `internal/tokenstorelive/live_test.go` (`TOKENSTORE_LIVE=1`); unit tests run
  against the library's mock backend
  (`internal/services/tokenstore_test.go`). The live test lives in its own
  package because the mock is process-global and must never shadow a live
  run.
- Go surface: `internal/services/tokenstore.go::KeychainTokenStore` behind the
  `internal/services/tokenstore.go::TokenStore` interface; bindings
  `bindings.go::StoreAccountTokens`, `bindings.go::LoadAccountTokens`,
  `bindings.go::DeleteAccountTokens`, `bindings.go::KeychainStatus`.
- Frontend storage schema bumped to 3
  (`frontend/src/accountStorage.ts`): localStorage holds metadata only —
  every write goes through `stripTokens`, which constructs the persisted
  record field by field so secrets cannot leak back in by accident. Tokens
  are hydrated from the keychain at restore (`restoreLastUsedAccount`),
  account selection (`AccountManager::handleSelectAccount`) and written back
  there after refresh.
- Migration semantics (schema 2 → 3), all in
  `frontend/src/accountStorage.ts::migrateAccountsIfNeeded`:
  - raw payload backed up before rewrite (existing pattern);
  - offline accounts removed (notice queued), authlib-injector convergence
    rules folded in so schema-1 stores migrate in one pass;
  - each surviving token pair is stored in the keychain **before**
    localStorage is rewritten; any keychain failure aborts the whole
    migration, leaves the old records untouched and surfaces
    `keychainError` — retry on next start, no account and no token lost;
  - memoized in-flight promise makes concurrent first calls safe;
  - idempotent once schema 3 is stamped.
- Degradation is explicit, never silent: keychain unavailable at startup →
  persistent danger callout (`app.keychainUnavailable`); login whose tokens
  cannot be stored → session still works in-memory plus warning
  (`app.tokenStorageWarning`); account whose keychain entry is missing at
  selection → "sign in again" notice, never a dead selection.

### Disclaimer

- `frontend/src/App.tsx` renders the Usage-Guidelines disclaimer
  (`app.disclaimer`: "NOT AN OFFICIAL MINECRAFT PRODUCT. NOT APPROVED BY OR
  ASSOCIATED WITH MOJANG OR MICROSOFT.") permanently at the bottom of the
  sidebar.

## Consequences

- Users with offline accounts lose them on upgrade (one-shot notice names
  them); there is no offline fallback anymore — intentional, per the product
  direction.
- On machines without a secret service the migration refuses to run and the
  user sees the reason; tokens stay in legacy storage until the platform is
  fixed. This is deliberate: silent plaintext fallback after promising
  keychain storage would be worse than a loud degradation.
- The keychain entry per account is keyed by the stable account id; deleting
  an account deletes its entry (best-effort — deletion must not fail on an
  already-clean keychain).
- Costs: one keychain round-trip per restore/selection; a new Go dependency;
  the frontend test doubles must mock five bindings instead of two.
- Microsoft login remains unimplemented by design; the contract draft for its
  server endpoints lives beside ADR-0007 and is not committed to HA-Contract
  yet (author instruction).

## Evidence

- Keychain selection and live verification: `internal/services/tokenstore.go`
  (interface + keychain implementation), `internal/services/tokenstore_test.go`
  (mock-backend round-trip), `internal/tokenstorelive/live_test.go` (live
  round-trip, passed with `TOKENSTORE_LIVE=1` on the author's KDE session,
  2026-11-08).
- Offline removal: `frontend/src/components/LoginView.tsx` (single login
  path), `internal/services/mc_services.go::resolveUserType` (no `legacy`
  output), `frontend/src/components/LoginView.test.tsx` (asserts no offline
  path remains).
- Migration evidence (Vitest, 2026-11-08): schema-2 fixture with an hrpauth,
  an offline and a legacy record → keychain stores exactly the hrpauth pair,
  offline removed with notice, localStorage metadata carries no token fields;
  keychain failure leaves schema-2 storage byte-identical; retry completes
  (`frontend/src/accountStorage.test.ts`).
- Disclaimer: `frontend/src/App.tsx` sidebar footer, i18n key
  `app.disclaimer` (`frontend/src/i18n.ts`).
- Full suite at commit time: `npm run lint` clean, `npm run typecheck` clean,
  `npm test` 47/47, `npm run build` ok, `go build/vet/test -race ./...` all
  green, knowledge anchor checker 0 problems.
- External references (competitor incidents, Usage Guidelines clause): see
  ADR-0007 context and the 2026-11-08 investigation reports — not re-verified
  here.
