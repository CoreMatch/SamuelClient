# ADR-0002: Authentication Is Pure Yggdrasil via authlib-injector

- **Status**: accepted
- **Date**: 2026-11-04

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

- Authenticate call: `internal/services/mc_services.go:206`
  (`{server}/authserver/authenticate`), request body `:70-79,208-215`; login
  handler `internal/handlers/mc_handlers.go:61-78`; frontend login
  `frontend/src/views/LoginView.tsx:157-165`.
- Refresh/validate: `internal/services/mc_services.go:266,273-275` (refresh,
  optional `selectedProfile.id`), `:317,343-347` (validate, 204 valid / 403
  invalid).
- Default auth server: `frontend/src/views/LoginView.tsx:48,51`
  (`https://backend.auth.samuelcheston.com/`); introduced/changed by `0ad13eb`
  and `61138ee` (`DEFAULT_HRPAUTH_URL`, "Login with HRPAUTH").
- authlib-injector: metadata fetch with ALI redirect
  `internal/services/mc_services.go:113-156`; JVM injection
  `:1233-1234` (`-javaagent`) and `:1239-1241` (prefetched metadata); reference
  doc `docs/authlib-injector wiki.pdf`; introduced by `19fd6c3`.
- Endpoint ownership: HRPAuth-Yggdrasil-API `main.go:147` (meta) and
  `main.go:164-171` (`/authserver/...` routes) match the launcher's calls.
- Dead local endpoints / offline UUID mismatch: routes
  `internal/routes/routes.go:151-158`; frontend offline UUID
  `frontend/src/views/LoginView.tsx:133,392-409` vs backend
  `internal/handlers/mc_handlers.go:598-610`.
- Unimplemented Microsoft login: `internal/services/mc_services.go:1137-1146`
  (`resolveUserType` handles `msa`; no implementation).
- Commits: `19fd6c3`, `0ad13eb`, `61138ee`, `c08d863` (HRPAUTH support fix,
  `/auth/validate`, ALI meta, refresh with selectedProfile).
- Not verified: that `backend.auth.samuelcheston.com` actually routes to
  HRPAuth-Yggdrasil-API in deployment (no in-repo config proves it).
