import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  HRPAuthValidate,
  HRPAuthRefresh,
  StoreAccountTokens,
  StoreMicrosoftAccountTokens,
  LoadAccountTokens,
  DeleteAccountTokens,
} from '../wailsjs/go/main/App';
import {
  ACCOUNTS_STORAGE_KEY,
  StoredAccount,
  restoreLastUsedAccount,
  migrateAccountsIfNeeded,
  saveAccount,
  consumeOfflineRemovedNotice,
  deleteAccountTokens,
} from './accountStorage';

// The Wails bindings are replaced with test doubles; account storage must
// never reach a real backend or a real keychain.
vi.mock('../wailsjs/go/main/App', () => ({
  HRPAuthValidate: vi.fn(),
  HRPAuthRefresh: vi.fn(),
  StoreAccountTokens: vi.fn(),
  StoreMicrosoftAccountTokens: vi.fn(),
  LoadAccountTokens: vi.fn(),
  DeleteAccountTokens: vi.fn(),
}));

const mockValidate = vi.mocked(HRPAuthValidate);
const mockRefresh = vi.mocked(HRPAuthRefresh);
const mockStore = vi.mocked(StoreAccountTokens);
const mockStoreMicrosoft = vi.mocked(StoreMicrosoftAccountTokens);
const mockLoad = vi.mocked(LoadAccountTokens);
const mockDelete = vi.mocked(DeleteAccountTokens);

// Schema-2 record shape: tokens inline in localStorage, offline allowed.
const schema2Raw = JSON.stringify([
  {
    id: '1',
    username: 'alice',
    uuid: 'p1',
    accessToken: 'at-alice',
    clientToken: 'ct-alice',
    authType: 'hrpauth',
    lastUsed: true,
  },
  {
    id: '2',
    username: 'bob',
    uuid: 'offline-uuid',
    accessToken: 'offline-uuid',
    clientToken: 'ct-bob',
    authType: 'offline',
  },
  {
    id: '3',
    username: 'carol',
    uuid: 'p3',
    accessToken: 'at-carol',
    clientToken: 'ct-carol',
    authType: 'hrpauth',
    legacy: true,
  },
]);

function readStored(): Array<Record<string, unknown>> {
  return JSON.parse(localStorage.getItem(ACCOUNTS_STORAGE_KEY) ?? '[]');
}

beforeEach(() => {
  vi.clearAllMocks();
  localStorage.clear();
  mockStore.mockResolvedValue({ success: true, data: null, error: '' });
  mockStoreMicrosoft.mockResolvedValue({ success: true, data: null, error: '' });
  mockDelete.mockResolvedValue({ success: true, data: null, error: '' });
});

describe('migrateAccountsIfNeeded (schema 2 -> 3)', () => {
  it('moves hrpauth tokens to the keychain, removes offline accounts, strips tokens from localStorage', async () => {
    localStorage.setItem(ACCOUNTS_STORAGE_KEY, schema2Raw);

    const outcome = await migrateAccountsIfNeeded();

    expect(outcome.done).toBe(true);
    expect(outcome.removedOffline).toEqual(['bob']);
    expect(outcome.keychainError).toBe('');

    // Only the usable hrpauth account's tokens move to the keychain — the
    // legacy record's tokens are dropped, never stored.
    expect(mockStore).toHaveBeenCalledTimes(1);
    expect(mockStore).toHaveBeenCalledWith('1', 'at-alice', 'ct-alice');

    const stored = readStored();
    expect(stored).toHaveLength(2);
    expect(stored[0]).toMatchObject({ id: '1', username: 'alice', authType: 'hrpauth' });
    expect(stored[1]).toMatchObject({ id: '3', username: 'carol', legacy: true });
    for (const record of stored) {
      expect(record.accessToken).toBeUndefined();
      expect(record.clientToken).toBeUndefined();
    }

    // The offline removal notice is queued exactly once...
    expect(consumeOfflineRemovedNotice()).toEqual(['bob']);
    // ...and consumed.
    expect(consumeOfflineRemovedNotice()).toBeNull();

    expect(localStorage.getItem('accounts_schema')).toBe('3');
    const backupKeys = Object.keys(localStorage).filter(k => k.startsWith(`${ACCOUNTS_STORAGE_KEY}.backup-`));
    expect(backupKeys.length).toBe(1);
  });

  it('is idempotent: a second run stores nothing new and adds no backup', async () => {
    localStorage.setItem(ACCOUNTS_STORAGE_KEY, schema2Raw);

    await migrateAccountsIfNeeded();
    const outcome = await migrateAccountsIfNeeded();

    expect(outcome.done).toBe(true);
    expect(outcome.removedOffline).toEqual([]);
    expect(mockStore).toHaveBeenCalledTimes(1); // still only the first run
    const backupKeys = Object.keys(localStorage).filter(k => k.startsWith(`${ACCOUNTS_STORAGE_KEY}.backup-`));
    expect(backupKeys.length).toBe(1);
  });

  it('leaves legacy storage untouched when the keychain rejects a token (loss-free rollback)', async () => {
    localStorage.setItem(ACCOUNTS_STORAGE_KEY, schema2Raw);
    localStorage.setItem('accounts_schema', '2');
    mockStore.mockResolvedValue({ success: false, data: null, error: 'keychain locked' });

    const outcome = await migrateAccountsIfNeeded();

    expect(outcome.done).toBe(false);
    expect(outcome.keychainError).toBe('keychain locked');
    // Old records intact: tokens still inline, offline still present, schema
    // unchanged — the migration will retry on next start.
    expect(JSON.parse(localStorage.getItem(ACCOUNTS_STORAGE_KEY) ?? '[]')).toHaveLength(3);
    expect(readStored()[0].accessToken).toBe('at-alice');
    expect(localStorage.getItem('accounts_schema')).toBe('2');
    expect(localStorage.getItem('offline_accounts_removed_v3')).toBeNull();
  });

  it('recovers on retry after the keychain becomes available', async () => {
    localStorage.setItem(ACCOUNTS_STORAGE_KEY, schema2Raw);
    localStorage.setItem('accounts_schema', '2');
    mockStore.mockResolvedValueOnce({ success: false, data: null, error: 'keychain locked' });

    const failed = await migrateAccountsIfNeeded();
    expect(failed.done).toBe(false);

    mockStore.mockResolvedValue({ success: true, data: null, error: '' });
    const retried = await migrateAccountsIfNeeded();
    expect(retried.done).toBe(true);
    expect(retried.removedOffline).toEqual(['bob']);
    expect(readStored()[0].accessToken).toBeUndefined();
    expect(localStorage.getItem('accounts_schema')).toBe('3');
  });

  it('treats empty storage as a fresh schema-3 install', async () => {
    const outcome = await migrateAccountsIfNeeded();
    expect(outcome).toEqual({ done: true, removedOffline: [], keychainError: '' });
    expect(localStorage.getItem('accounts_schema')).toBe('3');
    expect(mockStore).not.toHaveBeenCalled();
  });
});

describe('restoreLastUsedAccount (schema 3)', () => {
  const metadata: StoredAccount[] = [
    { id: '1', username: 'alice', uuid: 'p1', authType: 'hrpauth', lastUsed: true },
  ];

  beforeEach(() => {
    localStorage.setItem('accounts_schema', '3');
    localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(metadata));
  });

  it('hydrates tokens from the keychain and validates them', async () => {
    mockLoad.mockResolvedValue({
      success: true,
      data: { accessToken: 'kc-access', clientToken: 'kc-client' },
    });
    mockValidate.mockResolvedValue({ success: true, data: null });

    const result = await restoreLastUsedAccount();

    expect(mockLoad).toHaveBeenCalledWith('1');
    expect(mockValidate).toHaveBeenCalledWith('kc-access', 'kc-client');
    expect(result.needsReLogin).toBe(false);
    expect(result.account?.accessToken).toBe('kc-access');
    // localStorage never gains token fields.
    expect(readStored()[0].accessToken).toBeUndefined();
  });

  it('prompts re-login when the keychain has no entry for the account', async () => {
    mockLoad.mockResolvedValue({ success: false, data: null, error: 'token entry not found' });

    const result = await restoreLastUsedAccount();

    expect(result).toEqual({ account: null, needsReLogin: true, keychainError: '' });
    expect(mockValidate).not.toHaveBeenCalled();
  });

  it('refreshes a stale token and writes the refreshed pair back to the keychain', async () => {
    mockLoad.mockResolvedValue({
      success: true,
      data: { accessToken: 'kc-access', clientToken: 'kc-client' },
    });
    mockValidate.mockResolvedValue({ success: false, data: null, error: 'ForbiddenOperationException' });
    mockRefresh.mockResolvedValue({
      success: true,
      data: { accessToken: 'new-access', clientToken: 'new-client' },
    });

    const result = await restoreLastUsedAccount();

    expect(mockRefresh).toHaveBeenCalledWith('kc-access', 'kc-client', 'p1');
    expect(mockStore).toHaveBeenCalledWith('1', 'new-access', 'new-client');
    expect(result.needsReLogin).toBe(false);
    expect(result.account?.accessToken).toBe('new-access');
    // The refreshed tokens stay out of localStorage.
    expect(readStored()[0].accessToken).toBeUndefined();
  });

  it('prompts re-login when validation and refresh both fail', async () => {
    mockLoad.mockResolvedValue({
      success: true,
      data: { accessToken: 'kc-access', clientToken: 'kc-client' },
    });
    mockValidate.mockResolvedValue({ success: false, data: null, error: 'invalid token' });
    mockRefresh.mockResolvedValue({ success: false, data: null, error: 'refresh rejected' });

    const result = await restoreLastUsedAccount();

    expect(result.account).toBeNull();
    expect(result.needsReLogin).toBe(true);
  });

  it('skips a legacy last-used account instead of restoring it', async () => {
    localStorage.setItem(
      ACCOUNTS_STORAGE_KEY,
      JSON.stringify([{ id: '9', username: 'old', uuid: 'p9', authType: 'hrpauth', legacy: true, lastUsed: true }]),
    );

    const result = await restoreLastUsedAccount();

    expect(result.account).toBeNull();
    expect(result.needsReLogin).toBe(false);
    expect(mockLoad).not.toHaveBeenCalled();
  });

  it('reports the keychain error and asks for re-login when the migration cannot complete', async () => {
    // Roll back to a schema-2 record so the migration runs during restore.
    localStorage.setItem('accounts_schema', '2');
    localStorage.setItem(ACCOUNTS_STORAGE_KEY, schema2Raw);
    mockStore.mockResolvedValue({ success: false, data: null, error: 'secret service down' });

    const result = await restoreLastUsedAccount();

    expect(result.account).toBeNull();
    expect(result.needsReLogin).toBe(true);
    expect(result.keychainError).toBe('secret service down');
    // Nothing was lost.
    expect(readStored()).toHaveLength(3);
  });
});

describe('saveAccount (schema 3)', () => {
  beforeEach(() => {
    localStorage.setItem('accounts_schema', '3');
  });

  it('stores tokens in the keychain and keeps localStorage metadata-only', async () => {
    const { account, tokenStorageError } = await saveAccount({
      username: 'dave',
      uuid: 'p4',
      accessToken: 'at-dave',
      clientToken: 'ct-dave',
      authType: 'hrpauth',
      lastUsed: true,
    });

    expect(tokenStorageError).toBe('');
    expect(mockStore).toHaveBeenCalledWith(account.id, 'at-dave', 'ct-dave');
    const stored = readStored();
    expect(stored).toHaveLength(1);
    expect(stored[0]).toMatchObject({ username: 'dave', authType: 'hrpauth' });
    expect(stored[0].accessToken).toBeUndefined();
    expect(stored[0].clientToken).toBeUndefined();
  });

  it('dedupes by username+authType and keeps the stable id', async () => {
    const first = await saveAccount({
      username: 'dave',
      uuid: 'p4',
      accessToken: 'at-1',
      clientToken: 'ct-1',
      authType: 'hrpauth',
    });
    await saveAccount({
      username: 'dave',
      uuid: 'p4',
      accessToken: 'at-2',
      clientToken: 'ct-2',
      authType: 'hrpauth',
    });

    const stored = readStored();
    expect(stored).toHaveLength(1);
    expect(stored[0].id).toBe(first.account.id);
    expect(mockStore).toHaveBeenLastCalledWith(first.account.id, 'at-2', 'ct-2');
  });

  it('reports a token storage failure without losing the metadata', async () => {
    mockStore.mockResolvedValue({ success: false, data: null, error: 'keychain locked' });

    const { tokenStorageError } = await saveAccount({
      username: 'erin',
      uuid: 'p5',
      accessToken: 'at-erin',
      clientToken: 'ct-erin',
      authType: 'hrpauth',
    });

    expect(tokenStorageError).toBe('keychain locked');
    expect(readStored()).toHaveLength(1);
  });

  it('routes Microsoft accounts through the triple-token keychain binding (ADR-0007)', async () => {
    const { account, tokenStorageError } = await saveAccount({
      username: 'SteveMS',
      uuid: 'uuid-ms',
      accessToken: 'at-ms',
      clientToken: 'ct-ms',
      authType: 'microsoft',
      msaRefreshToken: 'msa-refresh',
    });

    expect(tokenStorageError).toBe('');
    expect(mockStoreMicrosoft).toHaveBeenCalledWith(account.id, 'at-ms', 'ct-ms', 'msa-refresh');
    expect(mockStore).not.toHaveBeenCalled();
    const stored = readStored();
    expect(stored).toHaveLength(1);
    expect(stored[0]).toMatchObject({ username: 'SteveMS', authType: 'microsoft' });
    // Neither session tokens nor the MSA refresh token may reach localStorage.
    expect(stored[0].accessToken).toBeUndefined();
    expect(stored[0].clientToken).toBeUndefined();
    expect((stored[0] as Record<string, unknown>).msaRefreshToken).toBeUndefined();
  });
});

describe('deleteAccountTokens', () => {
  it('forwards to the keychain binding', async () => {
    await deleteAccountTokens('acc-9');
    expect(mockDelete).toHaveBeenCalledWith('acc-9');
  });

  it('never throws when the binding rejects', async () => {
    mockDelete.mockRejectedValue(new Error('backend gone'));
    await expect(deleteAccountTokens('acc-9')).resolves.toBeUndefined();
  });
});
