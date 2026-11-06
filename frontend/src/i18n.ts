// Minimal catalog-based i18n. English is the only locale today; every
// user-facing string of the match handoff feature lives in the catalog so a
// future locale only needs a new table. t() falls back to the key itself if
// a catalog entry is missing, which keeps rendering safe.

const en = {
  'common.retry': 'Retry',
  'common.loadError': "Couldn't load {data}. Check your connection and try again.",
  'nav.play': 'Play',
  'nav.versions': 'Versions',
  'nav.accounts': 'Accounts',
  'nav.settings': 'Settings',
  'versions.tab.install': 'Install',
  'versions.tab.installed': 'Installed',
  'versions.refresh': 'Refresh',
  'versions.filterType': 'Filter by Type',
  'versions.emptyTitle': 'No versions installed',
  'versions.emptyHint': 'Use the Install tab to set up your first Minecraft version.',
  'versions.browse': 'Install a version',
  'versions.listData': 'the version list',
  'versions.installedChip': 'Installed',
  'versions.recommended': 'Recommended for Revolution: {versions}',
  'wizard.step.version': 'Select version',
  'wizard.step.loader': 'Select loader',
  'wizard.step.confirm': 'Confirm & install',
  'wizard.next': 'Next',
  'wizard.back': 'Back',
  'wizard.install': 'Install',
  'wizard.installing': 'Installing…',
  'wizard.installAnother': 'Install another',
  'wizard.vanilla': 'Vanilla',
  'wizard.vanillaHint': 'Plain Minecraft without a mod loader.',
  'wizard.fabricHint': 'Lightweight mod loader for modern mods.',
  'wizard.forgeHint': 'Classic mod loader with the largest catalog.',
  'wizard.loaderVersionsData': 'the loader list',
  'wizard.summary': 'Minecraft {version} with {loader}.',
  'wizard.summaryVanilla': 'Minecraft {version} (vanilla).',
  'wizard.installSuccess': 'Installed {version}. It is ready to play.',
  'wizard.installFailed': 'Installation failed: {error}',
  'install.step.jar': 'Game file',
  'install.step.library': 'Libraries',
  'install.step.native': 'Natives',
  'install.step.asset': 'Assets',
  'install.step.loader': 'Loader',
  'checklist.title': 'Get started',
  'checklist.signIn': 'Sign in',
  'checklist.signInHint': 'Add an account to play online or offline.',
  'checklist.installVersion': 'Install a version',
  'checklist.installVersionHint': 'Download Minecraft from the Versions page.',
  'checklist.firstLaunch': 'Launch the game',
  'checklist.firstLaunchHint': 'Pick your version and press Play.',
  'settings.appearance': 'Appearance',
  'settings.theme': 'Theme',
  'settings.themeDark': 'Dark',
  'settings.themeLight': 'Light',
  'settings.folders': 'Folders',
  'settings.modsFolder': 'Open Mods Folder',
  'accounts.emptyTitle': 'No accounts',
  'accounts.emptyHint': 'Add an account to get started.',
  'handoff.title': 'Match found',
  'handoff.mode': 'Mode: {mode}',
  'handoff.map': 'Map: {map}',
  'handoff.server': 'Server: {host}:{port}',
  'handoff.players': 'Players',
  'handoff.expiresIn': 'Expires in {time}',
  'handoff.expired': 'This match handoff has expired. Dismiss it or ask for a new one.',
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
