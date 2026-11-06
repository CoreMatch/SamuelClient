// Minimal catalog-based i18n. English is the only locale today; every
// user-facing string of the match handoff feature lives in the catalog so a
// future locale only needs a new table. t() falls back to the key itself if
// a catalog entry is missing, which keeps rendering safe.

const en = {
  'handoff.title': 'Match found',
  'handoff.mode': 'Mode: {mode}',
  'handoff.map': 'Map: {map}',
  'handoff.server': 'Server: {host}:{port}',
  'handoff.joinNow': 'Join now',
  'handoff.joining': 'Joining match…',
  'handoff.dismiss': 'Dismiss',
  'handoff.retry': 'Retry',
  'handoff.ok': 'OK',
  'handoff.loginRequired': 'Log in to join this match',
  'handoff.login': 'Log in',
  'handoff.launchFailed': 'Could not join the match: {error}',
  'handoff.noVersion':
    'No installed game version matches this match. Install one from the Versions page, then retry.',
} as const;

export type I18nKey = keyof typeof en;

const catalogs: Record<string, Record<string, string>> = { en };

let locale = 'en';

export function setLocale(next: string): void {
  locale = next;
}

// t resolves a key against the active locale (falling back to English, then
// to the key itself) and substitutes {placeholders}.
export function t(key: I18nKey, vars?: Record<string, string | number>): string {
  const template = catalogs[locale]?.[key] ?? catalogs.en[key] ?? key;
  if (!vars) return template;
  return Object.entries(vars).reduce(
    (acc, [name, value]) => acc.split(`{${name}}`).join(String(value)),
    template,
  );
}
