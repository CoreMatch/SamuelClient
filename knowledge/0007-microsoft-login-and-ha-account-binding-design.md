# ADR-0007: Microsoft Login Mapped onto HA Accounts — Design (Implementation Gated on Azure Approval)

- **Status**: proposed (design frozen; Microsoft-login implementation waits for an approved Azure application; the client-side prep work of this entry — offline removal, keychain token storage, disclaimer — is implemented)

## Context

The product direction (author decision, 2026-11): SamuelClient accepts **only**
genuine Microsoft login or HA login, and both end on an HA account — "genuine
login is only a way to log into the HA account that the genuine account maps
to". Two prior investigations feed this design:

1. **HA-side capability** (investigation report, 2026-11-08): the ecosystem
   already owns the mapping substrate — `accounts.mojang_uuid` (unique,
   nullable) in HRPAuth-Yggdrasil-API, the proxy-register absorption chain
   (WinnerProxy three-stage `hasJoined` → `POST /internal/proxy-register`),
   claim (`POST /admin/claim-user`), force-bind (`POST /admin/force-bind`) and
   the self-service mbe window (`POST /player/mojang-bind-enable`). What is
   missing is a client-initiated Microsoft login entry.
2. **Competitor practice** (investigation report, 2026-11-08): HMCL and Prism
   Launcher both implement dual-channel Microsoft OAuth (authorization code
   with a local loopback callback, plus device code), the standard
   MSA→XBL→XSTS→Minecraft-Services chain, and per-XErr player-facing guidance.
   Both register their own Azure applications and both publicly refuse
   client-id reuse (HMCL injects the id only into signed official builds;
   Prism's repo comment demands derivative builds change the key). New
   applications must pass Mojang's manual allowlist review
   (`https://aka.ms/mce-reviewappid`, official article "Java Edition Game
   Service API Review or Application Process"); unapproved apps get 403
   "Invalid app registration" from `api.minecraftservices.com`. Public
   rejection cases of individual developers exist.

Constraints carried into the design:

- No Microsoft-login code ships until an Azure client id exists and the
  allowlist review passes. Everything before that gate is contract draft and
  client prep only.
- The HA-Contract draft below stays in this repository until the author
  decides to land it; it is **not** committed to HA-Contract yet (author
  instruction).
- Server-side join verification (`join`/`hasJoined`, WinnerProxy) must not
  change: after the exchange the client holds an ordinary Yggdrasil session.
- No shared/pooled genuine tokens, no reuse of third-party client ids (EULA /
  Microsoft Services Agreement red lines, and the stated reason for Mojang's
  review regime).

## Decision

### 1. Login state machine (client view)

| State | Meaning | Credentials held |
|---|---|---|
| `S0` logged out | no usable account | none |
| `S1` HA account | hrpauth Yggdrasil session valid | Yggdrasil access/client token (keychain) |
| `S2` genuine-bound | Microsoft identity bound to a claimed HA account (cbh=1); the session is that account's | MSA refresh token (keychain) + last-exchanged Yggdrasil tokens (keychain) |
| `S3` genuine-unclaimed (playable) | Microsoft identity absorbed as a pending-claim proxy account (cbh=0); the player plays normally, claiming is a later optional action | MSA refresh token (keychain) + last-exchanged Yggdrasil tokens (keychain) |

Identity responsibility split (author corrections, 2026-11): **the client does
not participate in UUID generation** — the in-game UUID is generated
automatically by the HA side at account registration (existing behavior), and
genuine login is only responsible for binding `mojang_uuid` onto an account.
Likewise the client never decides profile attributes such as skins — syncing
the official skin after a genuine binding is implemented by the
HRPAuth-Yggdrasil-API side; the client is not involved. All tokens are opaque
to the client; it never parses or validates them.

Transitions:

- `S0 → S1`: today's HRPAuth username/password (`internal/services/mc_services.go::AuthenticateWithHRPAuth`).
- `S0 → S2`: Microsoft dual-channel login → `POST /authserver/microsoft`
  resolves the uuid to an already-claimed account → standard Yggdrasil
  session (`claimRequired=false`).
- `S0 → S3`: the exchange resolves the uuid to no claimed account — **the
  default path is pass-through, not a gate**: the server absorbs the identity
  with the existing proxy-register semantics (idempotent by `mojang_uuid`)
  and answers a playable session with `claimRequired=true`. The player is
  never blocked and never asked for HA credentials at this point.
- `S3 → S2`: claiming, an explicit optional action afterwards — in game via
  the existing claim chain (`/ha reg` → `POST /admin/claim-user`), or through
  the client's optional on-the-spot bind entry when the player voluntarily
  wants it (`POST /player/microsoft-bind`). Unclaimed stays `S3` and playable.
- `S2 → S0`: MSA refresh token rejected (`invalid_grant`) → full re-login.
- `S2 → S3`: unbind (`POST /player/microsoft-unbind`, WebUI or client) — the
  next exchange absorbs a fresh pending-claim account.
- `S1 → S2`: an existing HA user binds their genuine identity at any time
  (optional on-the-spot bind, no preceding exchange needed).

Visible consequences per path (UI copy lives in the failure-mode table below):

- **S3, the default**: the player enters the server normally with their
  genuine name; the official skin is served/synced by the Yggdrasil-API side
  (client not involved); the client shows claim guidance derived from
  `claimRequired`. Before claiming, the account is cbh=0: placeholder email
  `<username>@mojang-imported.invalid`, no full HA account features, and
  match records attach to this pending-claim account, which the 30-day bot
  cleanup can still remove if it is never claimed nor signed in. After
  claiming (cbh=1): real credentials, records and identity persist
  permanently, cleanup no longer applies. Claiming does not change the
  in-game identity — same profile before and after.
- **Optional on-the-spot bind (demoted path)**: players who already have an
  HA account and want to bind right away get an entry in the login dialog; it
  is purely voluntary and never a precondition for playing.

### 2. Client dual-channel flow and timeout degradation

Mirrors the competitor-validated pattern (HMCL `OAuth.java`, Prism
`MSALoginDialog`), adapted to this codebase:

1. **Primary — authorization code + PKCE (S256) + loopback**: open the system
   browser at `login.microsoftonline.com/consumers/oauth2/v2.0/authorize`
   (scope `XboxLive.signin offline_access`, `prompt=select_account`); a
   temporary loopback HTTP server on a candidate-port list receives
   `?code=&state=`; `state` mismatch is a hard failure; the server answers a
   success page and stops itself.
2. **Fallback — device code**: `POST .../consumers/oauth2/v2.0/devicecode`,
   show `user_code` + `verification_uri` (QR optional), poll the token
   endpoint honoring `interval` and `slow_down` (+5 s backoff), stop at
   `expires_in` (capped 15 min).
3. **Degradation ladder**: the dialog offers both; a browser flow idle beyond
   its wait budget offers switching to device code; `expired_token` /
   `authorization_declined` / user cancel return to `S0` with readable copy;
   network failure never silently loops (bounded retries per HTTP call, then
   error surface). Personal accounts on the device flow are asked to sign in
   twice by Microsoft (official docs) — the copy says so.

The chain after the MSA token (all server-to-server from the client process,
identical endpoints and payload shapes to the wiki.vg scheme and the HMCL/
Prism implementations):

- `POST https://user.auth.xboxlive.com/user/authenticate` with
  `RpsTicket: "d=<msa access token>"` → XBL token + `uhs`;
- `POST https://xsts.auth.xboxlive.com/xsts/authorize` with
  `RelyingParty: rp://api.minecraftservices.com/` → XSTS token; `uhs`
  consistency between both responses is enforced;
- `POST https://api.minecraftservices.com/authentication/login_with_xbox` with
  `identityToken: "XBL3.0 x=<uhs>;<xsts>"` → MC access token (24 h);
- `GET https://api.minecraftservices.com/minecraft/profile` → genuine uuid +
  name (404 handled per §7). The client uses this only to render what the
  player is about to sign in as; the identity that matters is the one the
  server re-fetches during the exchange (§3). All tokens stay opaque to the
  client — it never parses, validates, or derives identity from them.

XErr handling for the XSTS step follows the HMCL table (see §7).

### 3. Server-side re-verification of the MC token

The exchange/bind endpoints **never trust the client's claim**. Verification
points, in order:

1. The MC access token is presented to Mojang, not parsed locally: the server
   calls `GET https://api.minecraftservices.com/minecraft/profile` with
   `Authorization: Bearer <token>`. The token is issued for Mojang's audience
   — signature checks by us are out of scope; Mojang's answer is authoritative.
2. Non-200 → typed failure (401/403 → invalid token; 404 → profile missing,
   see §7; 5xx/timeout → upstream unavailable, retryable error).
3. On 200 the `id`/`name` pair becomes the only genuine identity used
   downstream; `id` is the `mojang_uuid` binding key.
4. The Mojang call is the sole consumer of the MC token: after the exchange
   the token is discarded; it is never stored, never returned, never logged
   (existing no-credential-logging discipline of the auth surface).
5. Rate protection: the exchange endpoint sits behind the same Redis rate
   bucket as `/authserver/authenticate`; an in-process short-TTL cache keyed by
   token hash shields Mojang from retry storms (implementation detail).
6. Absorption idempotence — one code path, one source of truth: an unbound
   uuid goes through the same `ProxyRegister` decision tree that WinnerProxy's
   stage 3 uses, and the unique `accounts.mojang_uuid` index is the single
   source of truth. Absorption at client login and absorption at server-side
   `hasJoined` are therefore the same semantics and cannot produce two
   accounts: whichever happens first creates the pending-claim account, and
   the other hits the idempotent "uuid already linked" branch and gets the
   same account back.

### 4. Unbind semantics

- `POST /player/microsoft-unbind`, self-service only (same token resolution as
  the bind endpoint): clears `accounts.mojang_uuid` of the caller's account.
  Idempotent — unbinding an unbound account succeeds.
- Unbinding never deletes the HA account or its profile; it only severs the
  genuine mapping.
- Consequence worth surfacing in UI copy: after unbind, joining an HA server
  with the genuine client makes WinnerProxy absorb the player again as a fresh
  cbh=0 proxy account (existing three-stage behavior) — the old HA account and
  the new proxy account coexist until force-bind/claim reconciles them.
- Operator-side transfer keeps using the existing
  `POST /admin/force-bind` (no change).

### 5. Token storage and expiry handling (client)

| Secret | Location | Lifetime handling |
|---|---|---|
| MSA refresh token | OS keychain (per-account entry) | 90-day rolling (Microsoft default); `invalid_grant` → drop to `S0`, prompt full re-login |
| Yggdrasil access/client token (from exchange or HRPAuth login) | OS keychain (per-account entry) | validate → refresh → re-auth ladder, unchanged from today's `frontend/src/accountStorage.ts::restoreLastUsedAccount` semantics |
| MC access token | memory only, single use | 24 h; never persisted |
| Account metadata (username, uuid, authType, id) | localStorage, schema 3 | not secret; survives token loss |

Keychain unavailable at runtime → migration keeps tokens where they are and
surfaces a persistent warning (implemented behavior, see ADR-0008 evidence);
new logins warn before storing.

### 6. HA-Contract draft

The endpoint contracts (paths, auth, request/response, error codes) are
drafted in `0007-microsoft-login-contract-draft.yaml` beside this entry.
Summary:

- `POST /authserver/microsoft` (no auth — the MC token is the credential;
  rate-limited like `/authserver/authenticate`): exchange. A uuid that is not
  linked to a claimed account is absorbed by default with the existing
  proxy-register semantics — being unbound is **not** an error. The response
  is the standard Yggdrasil authenticate shape plus `claimRequired` (true
  while the session's account is a cbh=0 pending-claim account).
- `POST /player/microsoft-bind` (Bearer, self-service): verified direct bind,
  the optional on-the-spot path for players who voluntarily bind, and for
  existing HA users linking their genuine identity; mbe remains for
  server-side absorption collisions.
- `POST /player/microsoft-unbind` (Bearer, self-service): idempotent clear.

Landing procedure when the Azure gate passes: register the endpoints in
HA-Contract `docs/api/openapi/hrpauth-yggdrasil-business.yaml` with a version
bump, run drift verification, then implement (standard contract-first rule).

### 7. Failure-mode list (player-facing)

| Cause (source) | Machine signal | Player-facing copy (zh) | Player-facing copy (en) |
|---|---|---|---|
| No Xbox profile (XErr 2148916233) | XSTS 401 | 该微软账号还没有 Xbox 档案。请先登录 minecraft.net 完成一次游戏档案创建，再回来登录。 | This Microsoft account has no Xbox profile yet. Sign in at minecraft.net once to create your game profile, then try again. |
| Region unavailable (XErr 2148916235) | XSTS 401 | 你所在地区无法使用 Xbox Live，无法完成正版登录。 | Xbox Live is not available in your region; genuine sign-in cannot complete. |
| Adult verification pending (XErr 2148916236/237) | XSTS 401 | 该账号需要在 Xbox 页面完成成人验证后才能登录。 | This account must complete adult verification on Xbox before signing in. |
| Child account (XErr 2148916238) | XSTS 401 | 该账号为儿童账号，需先加入一个由成人管理的微软家庭组。 | This is a child account; it must be added to a family managed by an adult first. |
| Xbox banned (XErr 2148916227) | XSTS 401 | 该账号可能被 Xbox 限制，无法完成正版登录。 | This account appears restricted by Xbox; genuine sign-in cannot complete. |
| No game profile on Mojang (profile 404, license present) | exchange 403 `minecraft_profile_missing` | 检测到游戏资格但还没有游戏档案：请先在 minecraft.net 登录一次创建档案。 | A license was found but no game profile exists. Sign in at minecraft.net once to create it. |
| No ownership and no profile (profile 404, no license) | exchange 403 `minecraft_license_not_found` | 该微软账号没有 Minecraft 购买记录（Game Pass 用户请先在官方启动器登录一次建档）。 | No Minecraft ownership found on this account (Game Pass users: launch the official launcher once to create a profile). |
| Refresh token expired/revoked (`invalid_grant`) | MSA token endpoint | 登录状态已过期，请重新登录微软账号。 | Your sign-in expired. Please sign in with Microsoft again. |
| Name collision, HA account not bindable | exchange 409 `username_already_bound` | 该正版用户名与一个未开放绑定的 HA 账号重名，无法自动落号：可等该账号在设置中开放绑定后重试，或联系管理员处理。 | That genuine name collides with an HA account that has not opened binding; automatic absorption is refused. Retry once that account opens binding, or contact an operator. |
| Bind target already bound to another uuid | bind 409 `mojang_uuid_already_bound` | 你的 HA 账号已绑定了另一个正版身份，请先解绑。 | Your HA account is already linked to a different genuine identity; unbind it first. |
| Uuid already on another HA account | bind 409 `mojang_uuid_bound_to_another_account` | 该正版身份已绑定了别的 HA 账号。如系本人，请用该账号登录后解绑，或联系管理员转移。 | This genuine identity is already linked to another HA account. If it is yours, unbind from that account or ask an operator to transfer it. |
| Mojang upstream unreachable | exchange/bind 502 | 正版验证服务暂时不可用，请稍后重试。 | Genuine verification is temporarily unavailable; try again later. |
| Keychain unavailable (local) | local | 本机凭据保险箱不可用，令牌将暂存旧位置并可能暴露，请尽快修复系统钥匙串。 | The OS credential store is unavailable; tokens stay in legacy storage and may be exposed. Fix the system keychain. |

## Consequences

- No data migration: `accounts.mojang_uuid` and its unique index exist; the
  claim/force-bind/mbe machinery is reused unchanged.
- Join verification is untouched: post-exchange sessions are ordinary
  Yggdrasil sessions; WinnerProxy keeps absorbing unbound genuine players
  server-side regardless of this feature.
- Costs: the Azure application + allowlist review is a hard external gate with
  no published SLA and known rejections; the exchange endpoint adds one
  Mojang round-trip per login (mitigated by the token-hash cache); unbind +
  re-absorb can create a duplicate proxy account (documented, reconcilable via
  force-bind).
- Author decisions and responsibility split recorded (2026-11, superseding
  the former open questions):
  - **Play first, bind later is the default** (former OQ-1): an unbound
    genuine player is never blocked and never asked for HA credentials at
    login — the exchange absorbs the identity (cbh=0) and answers a playable
    session with `claimRequired`; claiming stays an explicit optional action
    afterwards (in-game `/ha reg`, or the voluntary on-the-spot bind entry).
  - **UUID generation is not a client concern** (former OQ-2, closed): the
    in-game UUID is generated automatically by the HA side at account
    registration; genuine login only binds `mojang_uuid`.
  - **Skin sync is not a client concern** (former OQ-3, closed): syncing the
    official skin after a genuine binding is implemented by the
    HRPAuth-Yggdrasil-API side; the client is not involved.
  - No author-level open question remains; what is left is client-side
    implementation detail (rendering of the `claimRequired` guidance,
    placement of the voluntary bind entry).

## Evidence

- State-machine inputs: `accounts` model and bind chain —
  `/home/lnb/Desktop/HA/HRPAuth-Yggdrasil-API/models/models.go::Account`,
  `/home/lnb/Desktop/HA/HRPAuth-Yggdrasil-API/services/auth_service.go::ProxyRegister`,
  `/home/lnb/Desktop/HA/HRPAuth-Yggdrasil-API/services/admin_service.go::ClaimProxyAccount`,
  `/home/lnb/Desktop/HA/HRPAuth-Yggdrasil-API/services/admin_service.go::ForceBindTransfer`,
  `/home/lnb/Desktop/HA/HRPAuth-Yggdrasil-API/services/player_service.go::EnableMojangBind`.
- Server-side absorption: `/home/lnb/Desktop/HA/WinnerProxy/internal/handler/handler.go::HasJoined`
  (three-stage flow), `/home/lnb/Desktop/HA/WinnerProxy/internal/hrpauth/endpoints.go::RegisterByProxy`.
- Current client auth surface: `internal/services/mc_services.go::AuthenticateWithHRPAuth`,
  `frontend/src/accountStorage.ts::restoreLastUsedAccount`,
  `knowledge/0002-hrpauth-authentication-flow.md`.
- Microsoft chain and endpoints: wiki.vg "Microsoft Authentication Scheme"
  (community-verified) and Microsoft identity platform docs (device code,
  auth code, refresh-token lifetimes) — external, cited in the 2026-11-08
  investigation report; not re-verified here.
- Allowlist regime: Mojang support article "Java Edition Game Service API
  Review or Application Process" (help.minecraft.net, official) — external.
- Competitor patterns (dual channel, client-id policies, token storage,
  offline gating): HMCL/Prism source investigation, 2026-11-08 — external to
  this repo.
- Not verified: review approval likelihood for this project; device-code UX
  with `XboxLive.signin` on the consumers tenant (no first-hand test yet).
