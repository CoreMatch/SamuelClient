// Account persistence shared between views. Lives outside the component
// files so those only export React components (react-refresh requirement).

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
