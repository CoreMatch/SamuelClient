// InstalledVersion is the launcher's working copy of a Minecraft version
// (an installed vanilla or modded profile), shared across views.
export interface InstalledVersion {
  id: string;
  type: string;
  installed: boolean;
  gameDir?: string;
  loaderType?: string;
  loaderVersion?: string;
  inheritsFrom?: string;
}

export interface DiskVersion {
  id: string;
  type: string;
  has_json: boolean;
  has_jar: boolean;
  game_dir?: string;
  inherits_from?: string;
}

export function installedFromDisk(versions: DiskVersion[]): InstalledVersion[] {
  return versions.filter(v => v.has_json && v.has_jar).map(v => ({
    id: v.id,
    type: v.type || 'release',
    installed: true,
    ...(v.game_dir ? { gameDir: v.game_dir } : {}),
    ...(v.inherits_from ? { inheritsFrom: v.inherits_from } : {}),
  }));
}

export function versionKey(version: InstalledVersion): string {
  return JSON.stringify([version.id, version.gameDir ?? '']);
}

// HandoffPayload mirrors the INTEROP-SPEC v1 handoff document delivered by
// the matchmaking client. It never carries credentials; SamuelClient joins
// with its own logged-in account.
export interface HandoffJoinTarget {
  host: string;
  port: number;
  game_version?: string;
}

export interface HandoffPlayer {
  username: string;
  uuid: string;
}

export interface HandoffPayload {
  version: number;
  handoff_id: string;
  nonce?: string;
  created_at?: string;
  expires_at: string;
  match: {
    match_id: string;
    mode: string;
    map_name?: string;
    join_target: HandoffJoinTarget;
    players?: HandoffPlayer[];
  };
}
