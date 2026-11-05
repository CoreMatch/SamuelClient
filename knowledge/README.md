# Knowledge Base

## What This Is

This knowledge base is **SamuelClient's memo to itself**. It serves self-hosting
and maintenance of this repository: what the launcher is, how it authenticates
against the HA ecosystem, and where it is (and is not) covered by external
contracts. It is maintained alongside the code and is not meant to make external
projects accommodate this one.

## Order of Authority

This knowledge base **does not define outward behavior**. Outward behavior of
the HA ecosystem is defined by
[HA-Contract](https://github.com/CoreMatch/HA-Contract). As of this writing the
contract does **not** cover SamuelClient at all (see entry 0003); if a contract
later covers launcher behavior and an entry here contradicts it, **the contract
wins and the entry is stale — fix the entry, never the contract**, and surface
the conflict to the owner.

## Format

Each entry is one page in ADR style:

- **Status** (`accepted` / `superseded` / ...)
- **Context** — the problem and constraints
- **Decision** — what was chosen
- **Consequences** — including costs and limitations
- **Evidence** — `file:line`, protocol entries, commit hashes. Every piece of
  evidence must have been actually checked; anything that could not be verified
  is marked "not verified". Do not fabricate.

## Entries

| # | Title | Status |
|---|-------|--------|
| [0001](0001-wails-minecraft-launcher-positioning.md) | Wails-based Cross-Platform Minecraft Launcher | accepted |
| [0002](0002-yggdrasil-authlib-authentication-flow.md) | Authentication Is Pure Yggdrasil via authlib-injector | accepted |
| [0003](0003-zero-contract-coverage.md) | Zero Contract Coverage in HA-Contract | accepted |
| [0004](0004-local-http-boundary.md) | Local HTTP Boundary: Retire Direct Frontend→Gin HTTP in Favor of Wails Bindings | accepted |
