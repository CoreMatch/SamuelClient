export interface JavaRuntimeRecommendation {
  major: number;
  note: 'java17LtsFor117' | 'standard';
}

// Recommendations are informational. These ranges reflect Minecraft's
// documented runtime transitions and only return versions offered by the
// launcher's maintained LTS runtime installer.
export function recommendJavaMajor(gameVersion: string): JavaRuntimeRecommendation | null {
  const match = gameVersion.trim().match(/^v?(\d+)\.(\d+)(?:\.(\d+))?(?:[-+][\w.-]+)?$/i);
  if (!match) return null;

  const [, majorText, minorText, patchText = '0'] = match;
  const major = Number(majorText);
  const minor = Number(minorText);
  const patch = Number(patchText);

  if (major === 26 && minor >= 1) return { major: 25, note: 'standard' };
  if (major !== 1) return null;
  if (minor <= 16) return { major: 8, note: 'standard' };
  if (minor === 17) return { major: 17, note: 'java17LtsFor117' };
  if (minor >= 18 && minor <= 20) {
    if (minor === 20 && patch >= 5) return { major: 21, note: 'standard' };
    return { major: 17, note: 'standard' };
  }
  if (minor === 21) return { major: 21, note: 'standard' };
  return null;
}
