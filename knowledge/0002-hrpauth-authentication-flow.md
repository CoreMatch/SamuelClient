# ADR-0002: Authentication Is HRPAuth-only via authlib-injector

- **Status**: accepted

## Context

The launcher logs players in against the HA ecosystem's authentication. The HA
ecosystem separates general-purpose auth (HRPAuth core, OAuth2) from
Minecraft-specific auth (HRPAuth-Yggdrasil-API, Yggdrasil protocol). The
launcher uses the Minecraft-facing flavor so the game itself can authenticate,
and it uses exactly one provider: HRPAuth.

## Decision

SamuelClient authenticates **only against HRPAuth**, purely via the Yggdrasil
protocol (not OAuth2). There is no generic or user-configurable Yggdrasil
entry point anywhere in the product:

- The HRPAuth endpoint is built into the backend
  (`internal/services/mc_services.go::HRPAuthBaseURL`,
  `https://ha.mcnb.dev/yggdrasil-api`, fixed at deployment time). The HA
  ecosystem's authoritative API host is `ha.mcnb.dev`; the core relays the
  Yggdrasil routes under the `/yggdrasil-api` prefix, so the base URL carries
  that prefix (the core root answers `/authserver/*` only under it). No
  binding and no UI control accepts an auth server address.
- Account types are exactly two: `hrpauth` (username/password against
  HRPAuth) and `offline` (frontend-fabricated UUID).
- Login sends `POST {HRPAuth}/authserver/authenticate` with
  `username/password/clientToken/requestUser:true/agent:{Minecraft,1}`;
  refresh and validate use `/authserver/refresh` and `/authserver/validate`.
  These map one-to-one onto the routes served by HRPAuth-Yggdrasil-API.
- Game launch for `hrpauth` accounts fetches the HRPAuth metadata (honoring
  the `X-Authlib-Injector-API-Location` redirect) and injects
  `-javaagent:<authlib-injector.jar>=<HRPAuthBaseURL>` plus
  `-Dauthlibinjector.yggdrasil.prefetched=<Base64(metadata)>` into the launch
  command. Session verification (e.g. hasJoined) happens server-side in
  HRPAuth-Yggdrasil-API; because the injected URL is always the built-in
  HRPAuth endpoint, the launch/handoff/join chain cannot resolve to any other
  server.
- Stored accounts use schema version 2 (`accounts_schema` in localStorage).
  The one-time migration backs up the raw payload, re-types
  `authlib-injector` accounts whose server is empty or is HRPAuth to
  `hrpauth`, and keeps accounts of any other server as unselectable `legacy`
  records that the UI labels with a short notice.

## Consequences

- Bindings are server-less by construction: `HRPAuthLogin`,
  `HRPAuthRefresh`, `HRPAuthValidate` carry credentials/tokens only — the
  endpoint is not a parameter.
- Changing the HRPAuth address is a deployment decision (one Go value), not a
  user action.
- Legacy accounts of third-party Yggdrasil servers are preserved but inert;
  they never validate, launch, or auto-restore.

## Evidence

- Endpoint constant and type convergence:
  `internal/services/mc_services.go::HRPAuthBaseURL`,
  `internal/services/mc_services.go::IsHRPAuthAccount`.
- Auth calls: `internal/services/mc_services.go::AuthenticateWithHRPAuth`
  (`/authserver/authenticate`), `::RefreshHRPAuthTokenWithProfile`
  (optional `selectedProfile.id`), `::ValidateHRPAuthToken`
  (204 valid / 403 invalid); bindings `bindings.go::HRPAuthLogin`,
  `::HRPAuthRefresh`, `::HRPAuthValidate`.
- Metadata + injection: `internal/services/mc_services.go::getMetaWithRedirects`
  (with `::resolveALI`), `::GetHRPAuthMeta`, and
  `::BuildLaunchCommand` (`-javaagent` + prefetched metadata); launch-time
  prefetch `bindings.go::LaunchGame`.
- Frontend: `frontend/src/components/LoginView.tsx` (HRPAuth + offline
  only), `frontend/src/accountStorage.ts` (schema-2 migration, token
  validate/refresh restore).
- Endpoint ownership: HRPAuth-Yggdrasil-API
  (`/home/lnb/Desktop/HA/hrpauth/HRPAuth-Yggdrasil-API/main.go`, `/authserver/...`
  and meta routes) matches the launcher's calls.
- Deployment routing verified (2026-11-08, read-only probes of
  `https://ha.mcnb.dev`): the HRPAuth core answers `/health` and `/` at the
  root and relays the Yggdrasil surface under `/yggdrasil-api`
  (`POST /yggdrasil-api/authserver/authenticate` reaches the Yggdrasil-API,
  while the top-level `POST /authserver/authenticate` answers
  `route_not_found`). Hence the built-in base URL carries the
  `/yggdrasil-api` prefix. The core root and the relayed meta endpoint both
  nest the authlib-injector metadata under a `yggdrasil` key; the launcher
  unwraps it (`internal/services/mc_services.go::unwrapYggdrasilMeta`) before
  handing it to `-Dauthlibinjector.yggdrasil.prefetched`.
