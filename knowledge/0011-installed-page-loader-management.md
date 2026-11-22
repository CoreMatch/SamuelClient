# ADR-0011: Manage Game Loaders from Installed Versions

- **Status**: accepted

## Context

Fabric and Forge can be installed onto a Minecraft version after the base game
is already present. Loader profiles are separate launchable version entries
that inherit the base game's client jar. Loader installation and removal are
version-management tasks, not launch-time settings, so the Play page should
remain focused on selecting and launching a version.

## Decision

- Put loader management in **Versions → Installed**, reachable from each
  installed version row through **Manage loaders**.
- Let users choose Fabric or Forge and a compatible loader version, then
  install it into the same game directory as the selected version. If the
  selected row is already a loader profile, use its `inheritsFrom` Minecraft
  version so additional profiles are created as siblings.
- Allow removal of the selected Fabric or Forge profile after confirmation.
  Removal deletes only that profile's version directory and its matching
  `launcher_profiles.json` entry; it does not delete the base Minecraft
  version, shared libraries, or sibling loader profiles.
- Keep the Play page free of loader-management controls. It may display the
  selected profile's loader as read-only launch context.

## Consequences

- A game can keep multiple loader profiles and the vanilla profile side by
  side; each remains independently selectable and launchable.
- Removing a loader does not reclaim shared library files, avoiding damage to
  other profiles that may use them.
- Loader catalog access is performed when a user opens that version's manager,
  rather than during normal Play-page use.

## Evidence

- Installed-row manager entry and refresh behavior:
  `frontend/src/components/VersionManager.tsx:150-164,227-228`.
- Catalog, install, and remove actions:
  `frontend/src/components/VersionLoaderManager.tsx:34-40,56-83,85-115`.
- Loader-parent metadata exposed by disk scanning:
  `bindings.go:329-338,353-405`;
  frontend profile conversion in `frontend/src/types.ts:13-35`.
- Profile-only deletion and launcher profile cleanup:
  `internal/services/loader_profile.go:12-42,44-70`.
- Installed-page add/remove coverage:
  `frontend/src/components/VersionManager.test.tsx:119-201`;
  profile preservation coverage:
  `internal/services/loader_profile_test.go:10-74`.
- Play remains launch-focused with a read-only loader display:
  `frontend/src/components/HomeView.tsx:275-306,322,353-362`.
