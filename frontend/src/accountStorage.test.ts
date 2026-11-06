import { describe, it, expect, vi, beforeEach } from 'vitest';
import { HRPAuthValidate, HRPAuthRefresh } from '../wailsjs/go/main/App';
import {
  ACCOUNTS_STORAGE_KEY,
  StoredAccount,
  restoreLastUsedAccount,
  migrateAccountsIfNeeded,
} from './accountStorage';

// The Wails bindings are replaced with test doubles; account restore must
// never reach a real backend.
vi.mock('../wailsjs/go/main/App', () => ({
  HRPAuthValidate: vi.fn(),
  HRPAuthRefresh: vi.fn(),
}));

const mockValidate = vi.mocked(HRPAuthValidate);
const mockRefresh = vi.mocked(HRPAuthRefresh);

const hrpauthAccount: StoredAccount = {
  id: '1',
  username: 'alice',
  uuid: 'p1',
  accessToken: 'old-access',
  clientToken: 'old-client',
  authType: 'hrpauth',
  lastUsed: true,
};

const offlineAccount: StoredAccount = {
  id: '2',
  username: 'bob',
  uuid: 'offline-uuid',
  accessToken: 'offline-uuid',
  clientToken: 'ct',
  authType: 'offline',
  lastUsed: true,
};

function storeAccounts(accounts: StoredAccount[]) {
  localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(accounts));
}

describe('restoreLastUsedAccount', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    // Tests start from the current schema so restore focuses on token flow.
    localStorage.setItem('accounts_schema', '2');
  });

  it('returns no account and no re-login prompt when nothing is stored', async () => {
    const result = await restoreLastUsedAccount();
    expect(result).toEqual({ account: null, needsReLogin: false });
    expect(mockValidate).not.toHaveBeenCalled();
    expect(mockRefresh).not.toHaveBeenCalled();
  });

  it('returns an offline account as-is without calling any binding', async () => {
    storeAccounts([offlineAccount]);
    const result = await restoreLastUsedAccount();
    expect(result.needsReLogin).toBe(false);
    expect(result.account?.username).toBe('bob');
    expect(mockValidate).not.toHaveBeenCalled();
    expect(mockRefresh).not.toHaveBeenCalled();
  });

  it('keeps an HRPAuth account whose token is still valid', async () => {
    storeAccounts([hrpauthAccount]);
    mockValidate.mockResolvedValue({ success: true, data: null });

    const result = await restoreLastUsedAccount();

    expect(mockValidate).toHaveBeenCalledWith('old-access', 'old-client');
    expect(mockRefresh).not.toHaveBeenCalled();
    expect(result.needsReLogin).toBe(false);
    expect(result.account?.accessToken).toBe('old-access');
  });

  it('refreshes a stale HRPAuth token and persists the new tokens', async () => {
    storeAccounts([hrpauthAccount]);
    mockValidate.mockResolvedValue({ success: false, data: null, error: 'ForbiddenOperationException' });
    mockRefresh.mockResolvedValue({
      success: true,
      data: { accessToken: 'new-access', clientToken: 'new-client' },
    });

    const result = await restoreLastUsedAccount();

    expect(mockRefresh).toHaveBeenCalledWith('old-access', 'old-client', 'p1');
    expect(result.needsReLogin).toBe(false);
    expect(result.account?.accessToken).toBe('new-access');
    expect(result.account?.clientToken).toBe('new-client');

    // The refreshed tokens are written back to storage.
    const stored: StoredAccount[] = JSON.parse(localStorage.getItem(ACCOUNTS_STORAGE_KEY) ?? '[]');
    expect(stored[0].accessToken).toBe('new-access');
    expect(stored[0].clientToken).toBe('new-client');
  });

  it('prompts re-login when validation and refresh both fail', async () => {
    storeAccounts([hrpauthAccount]);
    mockValidate.mockResolvedValue({ success: false, data: null, error: 'invalid token' });
    mockRefresh.mockResolvedValue({ success: false, data: null, error: 'refresh rejected' });

    const result = await restoreLastUsedAccount();

    expect(result).toEqual({ account: null, needsReLogin: true });
  });

  it('treats a rejected validate call as an invalid token and still tries refresh', async () => {
    storeAccounts([hrpauthAccount]);
    mockValidate.mockRejectedValue(new Error('binding exploded'));
    mockRefresh.mockResolvedValue({
      success: true,
      data: { accessToken: 'new-access', clientToken: 'new-client' },
    });

    const result = await restoreLastUsedAccount();

    expect(mockRefresh).toHaveBeenCalled();
    expect(result.needsReLogin).toBe(false);
    expect(result.account?.accessToken).toBe('new-access');
  });

  it('skips a legacy last-used account instead of restoring it', async () => {
    storeAccounts([{ ...hrpauthAccount, authType: 'offline', legacy: true } as StoredAccount]);

    const result = await restoreLastUsedAccount();

    expect(result).toEqual({ account: null, needsReLogin: false });
    expect(mockValidate).not.toHaveBeenCalled();
  });
});

describe('migrateAccountsIfNeeded', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('re-types HRPAuth-bound authlib accounts, keeps others as legacy, and backs up once', () => {
    localStorage.setItem(
      ACCOUNTS_STORAGE_KEY,
      JSON.stringify([
        {
          id: '1',
          username: 'alice',
          uuid: 'p1',
          accessToken: 'at',
          clientToken: 'ct',
          authType: 'authlib-injector',
          authServer: 'https://backend.auth.samuelcheston.com/',
        },
        {
          id: '2',
          username: 'carol',
          uuid: 'p2',
          accessToken: 'at2',
          clientToken: 'ct2',
          authType: 'authlib-injector',
          authServer: 'https://other.yggdrasil.example/',
        },
        { id: '3', username: 'bob', uuid: 'p3', accessToken: 'a', clientToken: 'c', authType: 'offline' },
      ]),
    );

    migrateAccountsIfNeeded();

    const migrated = JSON.parse(localStorage.getItem(ACCOUNTS_STORAGE_KEY) ?? '[]');
    expect(migrated[0].authType).toBe('hrpauth');
    expect(migrated[0].authServer).toBeUndefined();
    expect(migrated[0].legacy).toBeUndefined();
    expect(migrated[1].legacy).toBe(true);
    expect(migrated[2].authType).toBe('offline');

    // A backup of the original payload exists.
    const backupKeys = Object.keys(localStorage).filter(k => k.startsWith(`${ACCOUNTS_STORAGE_KEY}.backup-`));
    expect(backupKeys.length).toBe(1);

    // Idempotent: running again adds no backup and changes nothing.
    migrateAccountsIfNeeded();
    const backupKeysAgain = Object.keys(localStorage).filter(k => k.startsWith(`${ACCOUNTS_STORAGE_KEY}.backup-`));
    expect(backupKeysAgain.length).toBe(1);
    expect(localStorage.getItem('accounts_schema')).toBe('2');
  });
});
