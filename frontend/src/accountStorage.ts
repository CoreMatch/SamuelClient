// Account persistence shared between views. Lives outside the component
// files so those only export React components (react-refresh requirement).

import { HRPAuthValidate, HRPAuthRefresh } from '../wailsjs/go/main/App';

// HRPAuth is the only online auth provider. Its host mirrors the backend's
// built-in services.HRPAuthBaseURL and is used solely to classify stored
// legacy accounts during migration — it is never user-editable.
const HRPAUTH_HOST = 'backend.auth.samuelcheston.com';

export interface StoredAccount {
  id: string;
  username: string;
  uuid: string;
  accessToken: string;
  clientToken: string;
  authType: 'offline' | 'hrpauth';
  lastUsed?: boolean;
  // Set by the one-time migration for stored accounts that pointed at a
  // non-HRPAuth Yggdrasil server: they stay visible but cannot be used.
  legacy?: boolean;
}

export const ACCOUNTS_STORAGE_KEY = 'minecraft_accounts';
const ACCOUNTS_SCHEMA_KEY = 'accounts_schema';
const ACCOUNTS_SCHEMA_VERSION = '2';

// migrateAccountsIfNeeded performs the one-time HRPAuth convergence of
// stored accounts (schema version guard makes it idempotent):
//   - a backup copy of the raw storage is written first,
//   - authlib-injector accounts whose server is empty or is HRPAuth are
//     re-typed to 'hrpauth',
//   - authlib-injector accounts pointing at any other server are kept but
//     marked legacy (the UI shows a short notice instead of using them).
export function migrateAccountsIfNeeded(): void {
  try {
    if (localStorage.getItem(ACCOUNTS_SCHEMA_KEY) === ACCOUNTS_SCHEMA_VERSION) {
      return;
    }
    const raw = localStorage.getItem(ACCOUNTS_STORAGE_KEY);
    if (raw) {
      localStorage.setItem(`${ACCOUNTS_STORAGE_KEY}.backup-${Date.now()}`, raw);

      interface LegacyStoredAccount extends Omit<StoredAccount, 'authType'> {
        authType: string;
        authServer?: string;
      }

      const accounts: LegacyStoredAccount[] = JSON.parse(raw);
      const migrated: StoredAccount[] = accounts.map(acc => {
        const base = {
          id: acc.id,
          username: acc.username,
          uuid: acc.uuid,
          accessToken: acc.accessToken,
          clientToken: acc.clientToken,
          lastUsed: acc.lastUsed,
        };
        if (acc.authType === 'authlib-injector') {
          const host = acc.authServer ? safeHost(acc.authServer) : '';
          if (!acc.authServer || host === HRPAUTH_HOST) {
            return { ...base, authType: 'hrpauth' as const };
          }
          // Unmappable server: keep the record, mark it unusable.
          return { ...base, authType: 'hrpauth' as const, legacy: true };
        }
        return { ...base, authType: acc.authType as StoredAccount['authType'] };
      });
      localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(migrated));
    }
    localStorage.setItem(ACCOUNTS_SCHEMA_KEY, ACCOUNTS_SCHEMA_VERSION);
  } catch (err) {
    console.error('Failed to migrate stored accounts:', err);
  }
}

function safeHost(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return '';
  }
}

// Save a new account to storage
export function saveAccount(account: Omit<StoredAccount, 'id'> & { id?: string }): StoredAccount {
  migrateAccountsIfNeeded();
  const stored = localStorage.getItem(ACCOUNTS_STORAGE_KEY);
  const accounts: StoredAccount[] = stored ? JSON.parse(stored) : [];

  const newAccount: StoredAccount = {
    username: account.username,
    uuid: account.uuid,
    accessToken: account.accessToken,
    clientToken: account.clientToken,
    authType: account.authType,
    lastUsed: account.lastUsed,
    id: account.id || Date.now().toString(),
  };

  // Check if account with same username and authType exists
  const existingIndex = accounts.findIndex(
    acc => acc.username === account.username && acc.authType === account.authType
  );

  if (existingIndex >= 0) {
    accounts[existingIndex] = { ...newAccount, id: accounts[existingIndex].id };
  } else {
    accounts.push(newAccount);
  }

  localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(accounts));
  return newAccount;
}

export interface RestoredAccount {
  account: StoredAccount | null;
  // True when a stored HRPAuth account failed both token validation and
  // refresh; the caller should prompt the user to log in again.
  needsReLogin: boolean;
}

// Restore the last-used account, silently validating HRPAuth tokens and
// refreshing them when stale (PENDING-DECISIONS #11). Offline accounts have
// no server to validate against and are returned as-is. Legacy accounts are
// skipped (they stay listed for the user to see). Never throws: any binding
// or storage error degrades to "no account" instead of breaking startup.
export async function restoreLastUsedAccount(): Promise<RestoredAccount> {
  migrateAccountsIfNeeded();
  let accounts: StoredAccount[] = [];
  try {
    const stored = localStorage.getItem(ACCOUNTS_STORAGE_KEY);
    if (stored) {
      accounts = JSON.parse(stored);
    }
  } catch (err) {
    console.error('Failed to load accounts:', err);
    return { account: null, needsReLogin: false };
  }

  const lastUsed = accounts.find(acc => acc.lastUsed && !acc.legacy);
  if (!lastUsed) {
    return { account: null, needsReLogin: false };
  }

  // Offline accounts have nothing to validate.
  if (lastUsed.authType !== 'hrpauth') {
    return { account: lastUsed, needsReLogin: false };
  }

  let valid = false;
  try {
    const result = await HRPAuthValidate(lastUsed.accessToken, lastUsed.clientToken);
    valid = result.success;
  } catch (err) {
    console.error('Failed to validate account token:', err);
  }
  if (valid) {
    return { account: lastUsed, needsReLogin: false };
  }

  // Token invalid or unreachable: try to refresh it.
  try {
    const refreshed = await HRPAuthRefresh(
      lastUsed.accessToken,
      lastUsed.clientToken,
      lastUsed.uuid,
    );
    if (refreshed.success && refreshed.data) {
      const data = refreshed.data as { accessToken?: string; clientToken?: string };
      const updated: StoredAccount = {
        ...lastUsed,
        accessToken: data.accessToken || lastUsed.accessToken,
        clientToken: data.clientToken || lastUsed.clientToken,
      };
      saveAccount(updated);
      return { account: updated, needsReLogin: false };
    }
  } catch (err) {
    console.error('Failed to refresh account token:', err);
  }

  return { account: null, needsReLogin: true };
}
