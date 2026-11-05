# ADR-0003: Zero Contract Coverage in HA-Contract

- **Status**: accepted

## Context

HA-Contract is the documentation center and single source of truth for the HA
ecosystem's outward behavior (API standards, OpenAPI specs, dev guides). A
question for maintainers is whether SamuelClient's behavior is governed by any
contract there.

## Decision

This is a statement of current fact rather than a chosen design: **HA-Contract
does not cover SamuelClient**. The launcher's outward behavior is not defined
by any contract in that repository.

## Consequences

- SamuelClient's external behavior (its auth calls) is currently defined only
  by its own code and this knowledge base, not by a shared contract. The Wails
  binding surface is internal to the application, not an outward API.
- If HA-Contract later adds coverage for the launcher and an entry here
  conflicts with it, the contract wins and this entry must be fixed (see the
  Order of Authority in the index).
- Cost/limitation: there is no contract gate preventing drift between the
  launcher and the auth services it calls; changes to the Yggdrasil endpoints
  it depends on are only caught at runtime/integration.

## Evidence

- HA-Contract positions itself as the ecosystem's contract center and lists its
  covered projects; SamuelClient is not among them: HA-Contract
  `/home/lnb/Desktop/HA/HA-Contract/README.md`
  (Project Structure lists HRPAuth, HRPAuth-Yggdrasil-API, HASkinLib,
  WinnerProxy, HASkinProxy, Cascade, HRPAuth-WebUI).
- Search of HA-Contract for this project returns no launcher contract:
  `grep -i "samuelclient|samuel client|samuel"` matches only
  `/home/lnb/Desktop/HA/HA-Contract/docs/dev/Cascade/README.md:24`
  (`SamuelCheston/RW-HPS`, an
  author/other-project reference, unrelated); `grep -i launcher` returns zero
  matches; "client" hits are unrelated contexts (Minecraft client, CSL client,
  clientToken).
- External protocol references (authlib-injector/CSL) are kept as immutable
  external standards in HA-Contract
  `/home/lnb/Desktop/HA/HA-Contract/docs/references/`, not as launcher
  behavior contracts.
- Not verified: none — this is a negative finding based on exhaustive grep of
  the HA-Contract working tree at its current `main`.
