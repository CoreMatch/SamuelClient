// InstalledVersion is the launcher's working copy of a Minecraft version
// (an installed vanilla or modded profile), shared across views.
export interface InstalledVersion {
  id: string;
  type: string;
  installed: boolean;
  loaderType?: string;
  loaderVersion?: string;
}
