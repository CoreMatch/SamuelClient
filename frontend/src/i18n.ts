// Minimal catalog-based i18n. English is the only locale today; every
// user-facing string lives in the catalog so a future locale only needs a new
// table. t() falls back to the key itself if a catalog entry is missing,
// which keeps rendering safe.

const en = {
  // Common
  'common.retry': 'Retry',
  'common.loadError': "Couldn't load {data}.",
  'common.cancel': 'Cancel',
  'common.delete': 'Delete',
  'common.close': 'Close',

  // Navigation
  'nav.play': 'Play',
  'nav.versions': 'Versions',
  'nav.accounts': 'Accounts',
  'nav.settings': 'Settings',
  'nav.signIn': 'Sign in',

  // Play page
  'play.version': 'Minecraft Version',
  'play.loader': 'Mod Loader',
  'play.java': 'Java',
  'play.account': 'Account',
  'play.noAccount': 'No account selected',
  'play.noVersions': 'No versions installed',
  'play.noJava': 'No Java installed',
  'play.none': 'None',
  'play.hrpauth': 'HRPAuth',
  'play.offline': 'Offline',
  'play.launchSettings': 'Launch Settings',
  'play.maxMemory': 'Maximum Memory',
  'play.jvmArgs': 'JVM Arguments',
  'play.width': 'Width',
  'play.height': 'Height',
  'play.logs': 'Logs',
  'play.noLogs': 'No logs',
  'play.play': 'Play',
  'play.launching': 'Launching…',
  'play.selectVersionAccount': 'Please select a version and account',
  'play.selectVersionFirst': 'Please select a Minecraft version first',
  'play.loaderInstallFailed': 'Failed to install loader. Please try again.',
  'play.launchFailed': 'Failed to launch game. Please try again.',

  // Versions page & install wizard
  'versions.tab.install': 'Install',
  'versions.tab.installed': 'Installed',
  'versions.refresh': 'Refresh',
  'versions.filterType': 'Filter by Type',
  'versions.emptyTitle': 'No versions installed',
  'versions.browse': 'Install a version',
  'versions.listData': 'the version list',
  'versions.installedChip': 'Installed',
  'versions.filter.all': 'All',
  'versions.filter.release': 'Release',
  'versions.filter.snapshot': 'Snapshot',
  'versions.filter.oldBeta': 'Old Beta',
  'versions.filter.oldAlpha': 'Old Alpha',
  'versions.deleteLabel': 'Remove {version} from the list',
  'wizard.step.version': 'Select version',
  'wizard.step.loader': 'Select loader',
  'wizard.step.confirm': 'Confirm & install',
  'wizard.next': 'Next',
  'wizard.back': 'Back',
  'wizard.install': 'Install',
  'wizard.installing': 'Installing…',
  'wizard.installAnother': 'Install another',
  'wizard.vanilla': 'Vanilla',
  'wizard.loaderVersion': 'Loader version',
  'wizard.loaderVersionsData': 'the loader list',
  'wizard.summary': 'Minecraft {version} with {loader}.',
  'wizard.summaryVanilla': 'Minecraft {version} (vanilla).',
  'wizard.installSuccess': 'Installed {version}.',
  'wizard.installFailed': 'Installation failed: {error}',
  'install.step.jar': 'Game file',
  'install.step.library': 'Libraries',
  'install.step.native': 'Natives',
  'install.step.asset': 'Assets',
  'install.step.loader': 'Loader',

  // First-run checklist
  'checklist.title': 'Get started',
  'checklist.signIn': 'Sign in',
  'checklist.installVersion': 'Install a version',
  'checklist.firstLaunch': 'Launch the game',

  // Settings
  'settings.folders': 'Folders',
  'settings.modsFolder': 'Open Mods Folder',
  'settings.java': 'Java Installations',
  'settings.addJava': 'Add Java',
  'settings.noJava': 'No Java installations',
  'settings.addJavaTitle': 'Add Java Installation',
  'settings.editJavaTitle': 'Edit Java Installation',
  'settings.javaPath': 'Java Path',
  'settings.friendlyName': 'Friendly Name',
  'settings.update': 'Update',
  'settings.deleteJavaTitle': 'Delete Java Installation',
  'settings.deleteJavaConfirm': 'Are you sure you want to delete this Java installation?',
  'settings.defaultJava': 'Default Java',
  'settings.setDefault': 'Set as default',
  'settings.edit': 'Edit',
  'settings.paths': 'Path Configuration',
  'settings.gameDir': 'Game Directory',
  'settings.notConfigured': 'Not configured',
  'settings.memory': 'Memory & JVM Settings',
  'settings.maxMemory': 'Maximum Memory (MB)',
  'settings.minMemory': 'Minimum Memory (MB)',
  'settings.javaArgs': 'Additional JVM Arguments',
  'settings.javaArgsLabel': 'Java Args',
  'settings.windowSize': 'Game Window Size',
  'settings.jvmPreview': 'JVM Args Preview',
  'settings.reset': 'Reset',
  'settings.save': 'Save',
  'settings.saving': 'Saving…',
  'settings.saved': 'Memory settings saved',
  'settings.failedSave': 'Failed to save settings',
  'settings.javaPathRequired': 'Java path is required',
  'settings.javaAddFailed': 'Failed to add Java installation',
  'settings.javaUpdateFailed': 'Failed to update Java installation',
  'settings.javaDeleteFailed': 'Failed to delete Java installation',
  'settings.javaDefaultFailed': 'Failed to set default Java',
  'settings.javaFetchFailed': 'Failed to fetch Java installations',
  'settings.err.minMemoryMin': 'Minimum memory must be at least {min} MB',
  'settings.err.maxMemoryMin': 'Maximum memory must be at least {min} MB',
  'settings.err.maxMemoryMax': 'Maximum memory cannot exceed {max} MB',
  'settings.err.minGtMax': 'Minimum memory cannot exceed maximum memory',
  'settings.err.widthRange': 'Game width must be between {min} and {max}',
  'settings.err.heightRange': 'Game height must be between {min} and {max}',

  // Accounts
  'accounts.add': 'Add Account',
  'accounts.emptyTitle': 'No accounts',
  'accounts.hrpauth': 'HRPAuth',
  'accounts.offline': 'Offline',
  'accounts.legacy': 'Legacy',
  'accounts.legacyNotice': 'Sign in again with HRPAuth',
  'accounts.deleteTitle': 'Delete Account',
  'accounts.deleteConfirm':
    'Are you sure you want to delete the account "{username}"? This action cannot be undone.',
  'accounts.select': 'Use account {username}',
  'accounts.lastUsed': 'Last used',

  // Login
  'login.title': 'Sign in',
  'login.authType': 'Authentication Type',
  'login.hrpauth': 'HRPAuth',
  'login.offline': 'Offline (Cracked)',
  'login.username': 'Username',
  'login.password': 'Password',
  'login.showPassword': 'Show password',
  'login.hidePassword': 'Hide password',
  'login.submit': 'Login',
  'login.manageAccounts': 'Manage Accounts',
  'login.usernameRequired': 'Username is required',
  'login.credentialsRequired': 'Username and password are required',
  'login.noProfile': 'No profile found',
  'login.failed': 'Login failed',
  'login.failedRetry': 'Login failed. Please try again.',

  // Match handoff (INTEROP-SPEC)
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
