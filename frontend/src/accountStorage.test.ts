import { describe, it, expect, vi, beforeEach } from 'vitest';
import { AuthlibValidate, AuthlibRefresh } from '../wailsjs/go/main/App';
import {
  ACCOUNTS_STORAGE_KEY,
  StoredAccount,
  restoreLastUsedAccount,
} from './accountStorage';

// The Wails bindings are replaced with test doubles; account restore must
// never reach a real backend.
vi.mock('../wailsjs/go/main/App', () => ({
  AuthlibValidate: vi.fn(),
  AuthlibRefresh: vi.fn(),
}));

const mockValidate = vi.mocked(AuthlibValidate);
const mockRefresh = vi.mocked(AuthlibRefresh);

const authlibAccount: StoredAccount = {
  id: '1',
  username: 'alice',
  uuid: 'p1',
  accessToken: 'old-access',
  clientToken: 'old-client',
  authType: 'authlib-injector',
  authServer: 'https://auth.example/',
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

  it('keeps an authlib account whose token is still valid', async () => {
    storeAccounts([authlibAccount]);
    mockValidate.mockResolvedValue({ success: true, data: null });

    const result = await restoreLastUsedAccount();

    expect(mockValidate).toHaveBeenCalledWith('https://auth.example/', 'old-access', 'old-client');
    expect(mockRefresh).not.toHaveBeenCalled();
    expect(result.needsReLogin).toBe(false);
    expect(result.account?.accessToken).toBe('old-access');
  });

  it('refreshes a stale authlib token and persists the new tokens', async () => {
    storeAccounts([authlibAccount]);
    mockValidate.mockResolvedValue({ success: false, data: null, error: 'ForbiddenOperationException' });
    mockRefresh.mockResolvedValue({
      success: true,
      data: { accessToken: 'new-access', clientToken: 'new-client' },
    });

    const result = await restoreLastUsedAccount();

    expect(mockRefresh).toHaveBeenCalledWith('https://auth.example/', 'old-access', 'old-client', 'p1');
    expect(result.needsReLogin).toBe(false);
    expect(result.account?.accessToken).toBe('new-access');
    expect(result.account?.clientToken).toBe('new-client');

    // The refreshed tokens are written back to storage.
    const stored: StoredAccount[] = JSON.parse(localStorage.getItem(ACCOUNTS_STORAGE_KEY) ?? '[]');
    expect(stored[0].accessToken).toBe('new-access');
    expect(stored[0].clientToken).toBe('new-client');
  });

  it('prompts re-login when validation and refresh both fail', async () => {
    storeAccounts([authlibAccount]);
    mockValidate.mockResolvedValue({ success: false, data: null, error: 'invalid token' });
    mockRefresh.mockResolvedValue({ success: false, data: null, error: 'refresh rejected' });

    const result = await restoreLastUsedAccount();

    expect(result).toEqual({ account: null, needsReLogin: true });
  });

  it('treats a rejected validate call as an invalid token and still tries refresh', async () => {
    storeAccounts([authlibAccount]);
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
});
