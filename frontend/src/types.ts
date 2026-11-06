// InstalledVersion is the launcher's working copy of a Minecraft version
// (an installed vanilla or modded profile), shared across views.
export interface InstalledVersion {
  id: string;
  type: string;
  installed: boolean;
  loaderType?: string;
  loaderVersion?: string;
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
