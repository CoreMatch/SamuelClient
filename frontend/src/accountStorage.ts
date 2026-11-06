// Account persistence shared between views. Lives outside the component
// files so those only export React components (react-refresh requirement).

import { AuthlibValidate, AuthlibRefresh } from '../wailsjs/go/main/App';

export interface StoredAccount {
  id: string;
  username: string;
  uuid: string;
  accessToken: string;
  clientToken: string;
  authType: 'offline' | 'authlib-injector';
  authServer?: string;
  lastUsed?: boolean;
}

export const ACCOUNTS_STORAGE_KEY = 'minecraft_accounts';

// Save a new account to storage
export function saveAccount(account: Omit<StoredAccount, 'id'> & { id?: string }): StoredAccount {
  const stored = localStorage.getItem(ACCOUNTS_STORAGE_KEY);
  const accounts: StoredAccount[] = stored ? JSON.parse(stored) : [];

  const newAccount: StoredAccount = {
    username: account.username,
    uuid: account.uuid,
    accessToken: account.accessToken,
    clientToken: account.clientToken,
    authType: account.authType,
    authServer: account.authServer,
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
  // True when a stored authlib-injector account failed both token validation
  // and refresh; the caller should prompt the user to log in again.
  needsReLogin: boolean;
}

// Restore the last-used account, silently validating authlib-injector tokens
// and refreshing them when stale (PENDING-DECISIONS #11). Offline accounts
// have no server to validate against and are returned as-is. Never throws:
// any binding or storage error degrades to "no account" instead of breaking
// startup.
export async function restoreLastUsedAccount(): Promise<RestoredAccount> {
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

  const lastUsed = accounts.find(acc => acc.lastUsed);
  if (!lastUsed) {
    return { account: null, needsReLogin: false };
  }

  // Offline accounts (or a missing server) have nothing to validate.
  if (lastUsed.authType !== 'authlib-injector' || !lastUsed.authServer) {
    return { account: lastUsed, needsReLogin: false };
  }

  let valid = false;
  try {
    const result = await AuthlibValidate(lastUsed.authServer, lastUsed.accessToken, lastUsed.clientToken);
    valid = result.success;
  } catch (err) {
    console.error('Failed to validate account token:', err);
  }
  if (valid) {
    return { account: lastUsed, needsReLogin: false };
  }

  // Token invalid or unreachable: try to refresh it.
  try {
    const refreshed = await AuthlibRefresh(
      lastUsed.authServer,
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
