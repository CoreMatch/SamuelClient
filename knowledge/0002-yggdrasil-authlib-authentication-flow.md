# ADR-0002: Authentication Is Pure Yggdrasil via authlib-injector

- **Status**: accepted
- **Date**: 2026-11-04

> **Supersession note (2026-11-05):** the authentication flow in this entry
> still stands, but its local-HTTP side does not: the local auth endpoints
> (`/auth/refresh`, `/auth/validate`, `/auth/offline`) and the backend's own
> offline-UUID implementation were retired with the Gin boundary by
> [ADR-0004](0004-local-http-boundary.md). Offline login now fabricates its
> UUID only in the frontend
> (`frontend/src/components/LoginView.tsx::generateOfflineUUID`); the
> backend's differing MD5-based algorithm is gone with the handlers package.
> The evidence line citing `internal/handlers/mc_handlers.go` carries a
> time-point anchor and is kept as the historical record.

## Context

The launcher must log players in against the HA ecosystem's authentication. The
HA ecosystem separates general-purpose auth (HRPAuth core, OAuth2) from
Minecraft-specific auth (HRPAuth-Yggdrasil-API, Yggdrasil protocol). The
launcher needs the Minecraft-facing flavor so the game itself can authenticate.

## Decision

SamuelClient authenticates **purely via the Yggdrasil protocol**, not OAuth2:

- Login sends `POST {server}/authserver/authenticate` with
  `username/password/clientToken/requestUser:true/agent:{Minecraft,1}`; refresh
  and validate use `/authserver/refresh` and `/authserver/validate`.
- These four endpoints map one-to-one onto the routes served by
  **HRPAuth-Yggdrasil-API** (`/authserver/{authenticate,refresh,validate,
  invalidate,signout}`), not the HRPAuth core's OAuth2 endpoints.
- "authlib support" means the **authlib-injector** protocol: the launcher
  fetches auth metadata (honoring the `X-Authlib-Injector-API-Location`
  redirect), then injects `-javaagent:<authlib-injector.jar>=<authServer>` and
  `-Dauthlibinjector.yggdrasil.prefetched=<Base64(metadata)>` into the game
  launch command.
- The default auth server is `https://backend.auth.samuelcheston.com/`.

## Consequences

- The launcher integrates with the Minecraft-specific microservice, consistent
  with HRPAuth core having split Yggdrasil out (see HRPAuth ADR-0001).
- Several local auth endpoints (`/auth/refresh`, `/auth/validate`,
  `/auth/offline`) are implemented in the backend but never called by the
  frontend; offline login fabricates a UUID locally, with an algorithm that
  differs between frontend and backend.
- A `microsoft`/`msa` auth type is declared in request types and a resolver but
  has **no Microsoft login implementation**.
- Cost: the mapping from the default domain to the Yggdrasil-API service is a
  deployment detail not evidenced in this repo (not verified).

## Evidence

- Authenticate call: `internal/services/mc_services.go::AuthenticateWithAuthlib`
  (`{server}/authserver/authenticate`), request body
  `internal/services/mc_services.go::AuthlibAuthRequest`; login
  binding `bindings.go::AuthlibLogin`; frontend login
  `frontend/src/components/LoginView.tsx:159-162`.
- Refresh/validate:
  `internal/services/mc_services.go::RefreshAuthlibTokenWithProfile`
  (refresh, optional `selectedProfile.id`),
  `internal/services/mc_services.go::ValidateAuthlibToken` (validate,
  204 valid / 403 invalid).
- Default auth server: `frontend/src/components/LoginView.tsx:49,52`
  (`https://backend.auth.samuelcheston.com/`); introduced/changed by `0ad13eb`
  and `61138ee` (`DEFAULT_HRPAUTH_URL`, "Login with HRPAUTH").
- authlib-injector: metadata fetch with ALI redirect
  `internal/services/mc_services.go::getMetaWithRedirects` (with
  `internal/services/mc_services.go::resolveALI`); JVM injection
  `internal/services/mc_services.go::BuildLaunchCommand` (`-javaagent` and
  prefetched metadata); reference
  doc `docs/authlib-injector wiki.pdf`; introduced by `19fd6c3`.
- Endpoint ownership: HRPAuth-Yggdrasil-API
  `/home/lnb/Desktop/HA/HRPAuth-Yggdrasil-API/main.go::Meta` (meta) and
  `/home/lnb/Desktop/HA/HRPAuth-Yggdrasil-API/main.go::authserver`
  (`/authserver/...` routes) match the launcher's calls.
- Dead local endpoints / offline UUID mismatch: routes
  `internal/routes/routes.go:L151-L158 @ 27745e6`; frontend offline UUID
  `frontend/src/components/LoginView.tsx:136,388-405` vs backend
  `internal/handlers/mc_handlers.go:L598-L610 @ 27745e6`.
- Unimplemented Microsoft login: `internal/services/mc_services.go::resolveUserType`
  (handles `msa`; no implementation).
- Commits: `19fd6c3`, `0ad13eb`, `61138ee`, `c08d863` (HRPAUTH support fix,
  `/auth/validate`, ALI meta, refresh with selectedProfile).
- Not verified: that `backend.auth.samuelcheston.com` actually routes to
  HRPAuth-Yggdrasil-API in deployment (no in-repo config proves it).
