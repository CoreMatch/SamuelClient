// Account persistence shared between views. Lives outside the component
// files so they only export React components (react-refresh requirement).
//
// Storage split (ADR-0008): localStorage keeps non-sensitive account
// metadata only (schema 3); session tokens live in the OS keychain behind the
// Go bindings (StoreAccountTokens / LoadAccountTokens / DeleteAccountTokens).
// Migration from schema 2 (tokens inline in localStorage, offline accounts
// present) is idempotent and loss-free: any keychain failure aborts the
// migration and leaves the old records untouched.

import {
  HRPAuthValidate,
  HRPAuthRefresh,
  StoreAccountTokens,
  LoadAccountTokens,
  DeleteAccountTokens,
} from '../wailsjs/go/main/App';

// HRPAuth is the only online auth provider. This is the HISTORICAL built-in
// host that pre-migration (schema<=2) accounts stored in `authServer`; it is
// used solely to classify those legacy accounts during migration — it is
// never user-editable. It deliberately stays the old host even though the
// active endpoint (services.HRPAuthBaseURL) moved to ha.mcnb.dev, so existing
// accounts are recognized as first-party rather than marked legacy.
const HRPAUTH_HOST = 'backend.auth.samuelcheston.com';

export interface StoredAccount {
  id: string;
  username: string;
  uuid: string;
  authType: 'hrpauth';
  lastUsed?: boolean;
  // Set by the migration for stored accounts that pointed at a non-HRPAuth
  // Yggdrasil server: they stay visible but cannot be used.
  legacy?: boolean;
  // Session tokens are in-memory only since schema 3 (ADR-0008). They are
  // hydrated from the OS keychain and must never be written back to
  // localStorage; stripTokens() guards every persistence path.
  accessToken?: string;
  clientToken?: string;
}

export const ACCOUNTS_STORAGE_KEY = 'minecraft_accounts';
const ACCOUNTS_SCHEMA_KEY = 'accounts_schema';
const ACCOUNTS_SCHEMA_VERSION = '3';
// One-shot notice listing the offline accounts removed by the schema-3
// migration, consumed by the app shell so the user is never left wondering
// where they went.
const OFFLINE_REMOVED_KEY = 'offline_accounts_removed_v3';

export interface MigrationOutcome {
  // True once schema 3 is in place (whether by this call or an earlier one).
  done: boolean;
  // Usernames of offline accounts removed by this migration run (empty when
  // none were present or the run was a no-op).
  removedOffline: string[];
  // Non-empty when the keychain rejected a token during migration; the old
  // storage was left untouched and the migration will retry next start.
  keychainError: string;
}

interface LegacyStoredAccount {
  id?: string;
  username?: string;
  uuid?: string;
  accessToken?: string;
  clientToken?: string;
  authType?: string;
  authServer?: string;
  lastUsed?: boolean;
  legacy?: boolean;
}

let migrationInFlight: Promise<MigrationOutcome> | null = null;

function safeHost(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return '';
  }
}

// stripTokens returns a copy of the record without any secret fields. It
// guards every localStorage write so a future code path cannot reintroduce
// plaintext tokens by accident: the persisted shape is constructed field by
// field, never by subtracting secrets from a wider object.
function stripTokens(account: StoredAccount): StoredAccount {
  const metadata: StoredAccount = {
    id: account.id,
    username: account.username,
    uuid: account.uuid,
    authType: account.authType,
  };
  if (account.lastUsed !== undefined) metadata.lastUsed = account.lastUsed;
  if (account.legacy !== undefined) metadata.legacy = account.legacy;
  return metadata;
}

function readRawAccounts(): LegacyStoredAccount[] {
  const raw = localStorage.getItem(ACCOUNTS_STORAGE_KEY);
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as LegacyStoredAccount[]) : [];
  } catch {
    return [];
  }
}

// migrateAccountsIfNeeded converges stored accounts on schema 3:
//   - a backup copy of the raw storage is written first (once),
//   - authlib-injector accounts whose server is empty or is HRPAuth are
//     re-typed to 'hrpauth'; any other server stays marked legacy,
//   - offline accounts are removed (their usernames land in a one-shot
//     notice),
//   - remaining tokens move into the OS keychain and are stripped from
//     localStorage.
// Any keychain failure aborts the whole migration without touching the
// existing storage, so a retry finds the same state and no account is lost.
export function migrateAccountsIfNeeded(): Promise<MigrationOutcome> {
  if (migrationInFlight) return migrationInFlight;
  migrationInFlight = runMigration().finally(() => {
    migrationInFlight = null;
  });
  return migrationInFlight;
}

async function runMigration(): Promise<MigrationOutcome> {
  const noop: MigrationOutcome = { done: true, removedOffline: [], keychainError: '' };
  if (localStorage.getItem(ACCOUNTS_SCHEMA_KEY) === ACCOUNTS_SCHEMA_VERSION) {
    return noop;
  }

  const raw = localStorage.getItem(ACCOUNTS_STORAGE_KEY);
  if (!raw) {
    localStorage.setItem(ACCOUNTS_SCHEMA_KEY, ACCOUNTS_SCHEMA_VERSION);
    return noop;
  }

  const legacyAccounts = readRawAccounts();

  // Step 1 — converge auth types (the schema-2 rules, folded in so a fresh
  // schema-1 store migrates in one pass).
  const converged: LegacyStoredAccount[] = legacyAccounts.map(acc => {
    if (acc.authType === 'authlib-injector') {
      const host = acc.authServer ? safeHost(acc.authServer) : '';
      if (!acc.authServer || host === HRPAUTH_HOST) {
        return { ...acc, authType: 'hrpauth' };
      }
      return { ...acc, authType: 'hrpauth', legacy: true };
    }
    return acc;
  });

  // Step 2 — offline accounts are no longer a supported login path
  // (ADR-0008); collect their names for the one-shot notice.
  const removedOffline = converged
    .filter(acc => acc.authType === 'offline' && !acc.legacy)
    .map(acc => acc.username ?? '');
  const kept = converged.filter(acc => !(acc.authType === 'offline' && !acc.legacy));

  // Step 3 — move surviving tokens into the keychain before touching
  // localStorage. Legacy (unusable) records drop their tokens without
  // storing them. Any failure aborts with the old storage intact.
  for (const acc of kept) {
    if (acc.legacy || !acc.accessToken) continue;
    try {
      const result = await StoreAccountTokens(acc.id ?? '', acc.accessToken, acc.clientToken ?? '');
      if (!result.success) {
        return { done: false, removedOffline: [], keychainError: result.error || 'keychain store failed' };
      }
    } catch (err) {
      return {
        done: false,
        removedOffline: [],
        keychainError: err instanceof Error ? err.message : 'keychain store failed',
      };
    }
  }

  // Step 4 — everything is persisted in the keychain; rewrite the metadata.
  localStorage.setItem(`${ACCOUNTS_STORAGE_KEY}.backup-${Date.now()}`, raw);
  const metadata: StoredAccount[] = kept.map(acc =>
    stripTokens({
      id: acc.id ?? Date.now().toString(),
      username: acc.username ?? '',
      uuid: acc.uuid ?? '',
      authType: 'hrpauth',
      lastUsed: acc.lastUsed,
      legacy: acc.legacy,
    }),
  );
  localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(metadata));
  localStorage.setItem(ACCOUNTS_SCHEMA_KEY, ACCOUNTS_SCHEMA_VERSION);
  if (removedOffline.length > 0) {
    localStorage.setItem(OFFLINE_REMOVED_KEY, JSON.stringify(removedOffline));
  }
  return { done: true, removedOffline, keychainError: '' };
}

// consumeOfflineRemovedNotice returns the usernames of offline accounts the
// migration removed, exactly once; later calls return null.
export function consumeOfflineRemovedNotice(): string[] | null {
  const raw = localStorage.getItem(OFFLINE_REMOVED_KEY);
  if (!raw) return null;
  localStorage.removeItem(OFFLINE_REMOVED_KEY);
  try {
    const names = JSON.parse(raw);
    return Array.isArray(names) ? names.filter(n => typeof n === 'string' && n !== '') : null;
  } catch {
    return null;
  }
}

export interface SaveAccountResult {
  account: StoredAccount;
  // Non-empty when the tokens could not be stored in the keychain. The
  // account metadata is still persisted and the session keeps working
  // in-memory; the caller should surface a warning.
  tokenStorageError: string;
}

// Save a new account to storage: tokens go to the keychain, metadata to
// localStorage.
export async function saveAccount(
  account: Omit<StoredAccount, 'id'> & { id?: string },
): Promise<SaveAccountResult> {
  await migrateAccountsIfNeeded();
  const accounts = readStoredMetadata();

  // Resolve the stable id first: an existing record with the same
  // username+authType keeps its id, and the keychain entry must be written
  // under exactly that id so metadata and tokens never diverge.
  const existing = accounts.find(
    acc => acc.username === account.username && acc.authType === account.authType,
  );
  const id = account.id || existing?.id || Date.now().toString();

  const newAccount: StoredAccount = {
    username: account.username,
    uuid: account.uuid,
    authType: account.authType,
    lastUsed: account.lastUsed,
    legacy: account.legacy,
    id,
    accessToken: account.accessToken,
    clientToken: account.clientToken,
  };

  let tokenStorageError = '';
  if (newAccount.accessToken) {
    try {
      const result = await StoreAccountTokens(newAccount.id, newAccount.accessToken, newAccount.clientToken ?? '');
      if (!result.success) {
        tokenStorageError = result.error || 'keychain store failed';
      }
    } catch (err) {
      tokenStorageError = err instanceof Error ? err.message : 'keychain store failed';
    }
  }

  const metadata = stripTokens(newAccount);
  if (existing) {
    const existingIndex = accounts.indexOf(existing);
    accounts[existingIndex] = { ...metadata, id: existing.id };
  } else {
    accounts.push(metadata);
  }

  localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(accounts));
  return { account: newAccount, tokenStorageError };
}

function readStoredMetadata(): StoredAccount[] {
  try {
    const stored = localStorage.getItem(ACCOUNTS_STORAGE_KEY);
    if (!stored) return [];
    const parsed = JSON.parse(stored);
    return Array.isArray(parsed) ? (parsed as StoredAccount[]) : [];
  } catch {
    return [];
  }
}

// persistAccountsMetadata writes metadata-only records (used by the account
// manager for selection/deletion); tokens never pass through it.
export function persistAccountsMetadata(accounts: StoredAccount[]): void {
  localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(accounts.map(stripTokens)));
}

// deleteAccountTokens removes the keychain entry of one account. Best-effort
// by design: deleting an account must not fail because the keychain is
// already clean or momentarily unreachable.
export async function deleteAccountTokens(id: string): Promise<void> {
  try {
    await DeleteAccountTokens(id);
  } catch {
    // swallow: metadata removal proceeds regardless
  }
}

// loadAccountTokens hydrates one account's tokens from the keychain; null
// when nothing is stored or the backend is unreachable.
export async function loadAccountTokens(
  id: string,
): Promise<{ accessToken: string; clientToken: string } | null> {
  try {
    const result = await LoadAccountTokens(id);
    if (result.success && result.data) {
      const data = result.data as { accessToken?: string; clientToken?: string };
      if (data.accessToken) {
        return { accessToken: data.accessToken, clientToken: data.clientToken ?? '' };
      }
    }
  } catch {
    // treated as "no tokens" below
  }
  return null;
}

export interface RestoredAccount {
  account: StoredAccount | null;
  // True when a stored HRPAuth account failed token loading, validation and
  // refresh; the caller should prompt the user to log in again.
  needsReLogin: boolean;
  // Non-empty when the migration could not move tokens into the keychain;
  // the caller should surface a warning (tokens are still in legacy storage
  // and no account was lost).
  keychainError: string;
}

// Restore the last-used account, silently validating HRPAuth tokens and
// refreshing them when stale (PENDING-DECISIONS #11). Tokens are read from
// and written back to the OS keychain; localStorage never sees them. Legacy
// accounts are skipped (they stay listed for the user to see). Never throws:
// any binding or storage error degrades to "no account" instead of breaking
// startup.
export async function restoreLastUsedAccount(): Promise<RestoredAccount> {
  const migration = await migrateAccountsIfNeeded();
  if (!migration.done) {
    // Tokens could not move to the keychain; the legacy records are intact.
    // Degrade to "no account" with a visible reason instead of reading
    // plaintext tokens from localStorage.
    return { account: null, needsReLogin: true, keychainError: migration.keychainError };
  }

  const accounts = readStoredMetadata();
  const lastUsed = accounts.find(acc => acc.lastUsed && !acc.legacy);
  if (!lastUsed) {
    return { account: null, needsReLogin: false, keychainError: '' };
  }

  const tokens = await loadAccountTokens(lastUsed.id);
  if (!tokens) {
    return { account: null, needsReLogin: true, keychainError: '' };
  }

  const hydrated: StoredAccount = { ...lastUsed, ...tokens };

  let valid = false;
  try {
    const result = await HRPAuthValidate(tokens.accessToken, tokens.clientToken);
    valid = result.success;
  } catch (err) {
    console.error('Failed to validate account token:', err);
  }
  if (valid) {
    return { account: hydrated, needsReLogin: false, keychainError: '' };
  }

  // Token invalid or unreachable: try to refresh it.
  try {
    const refreshed = await HRPAuthRefresh(tokens.accessToken, tokens.clientToken, hydrated.uuid);
    if (refreshed.success && refreshed.data) {
      const data = refreshed.data as { accessToken?: string; clientToken?: string };
      const nextAccess = data.accessToken || tokens.accessToken;
      const nextClient = data.clientToken || tokens.clientToken;
      // Write the refreshed pair back to the keychain; a failure only costs
      // the next-launch silent restore, never the current session.
      try {
        await StoreAccountTokens(hydrated.id, nextAccess, nextClient);
      } catch (err) {
        console.error('Failed to persist refreshed tokens:', err);
      }
      return {
        account: { ...hydrated, accessToken: nextAccess, clientToken: nextClient },
        needsReLogin: false,
        keychainError: '',
      };
    }
  } catch (err) {
    console.error('Failed to refresh account token:', err);
  }

  return { account: null, needsReLogin: true, keychainError: '' };
}
