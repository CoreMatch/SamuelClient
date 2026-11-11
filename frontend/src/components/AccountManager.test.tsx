import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import AccountManager from './AccountManager';
import { LoadAccountTokens, DeleteAccountTokens } from '../../wailsjs/go/main/App';
import { ACCOUNTS_STORAGE_KEY } from '../accountStorage';

// The Wails bindings are replaced with test doubles; the account manager
// must never reach a real backend or a real keychain. (accountStorage also
// imports the validate/refresh/store bindings — mock those too so nothing
// reaches a real backend.)
vi.mock('../../wailsjs/go/main/App', () => ({
  HRPAuthValidate: vi.fn(),
  HRPAuthRefresh: vi.fn(),
  StoreAccountTokens: vi.fn(),
  StoreMicrosoftAccountTokens: vi.fn(),
  LoadAccountTokens: vi.fn(),
  DeleteAccountTokens: vi.fn(),
}));

const mockLoad = vi.mocked(LoadAccountTokens);
const mockDelete = vi.mocked(DeleteAccountTokens);

// Schema-3 metadata: two usable accounts (one Microsoft) and one legacy.
const storedAccounts = [
  { id: 'a1', username: 'steve', uuid: 'u1', authType: 'hrpauth', lastUsed: true },
  { id: 'a2', username: 'SteveMS', uuid: 'u2', authType: 'microsoft' },
  { id: 'a3', username: 'oldy', uuid: 'u3', authType: 'hrpauth', legacy: true },
];

function seedAccounts(): void {
  localStorage.setItem('accounts_schema', '3');
  localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(storedAccounts));
}

describe('AccountManager', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    mockDelete.mockResolvedValue({ success: true, data: null, error: '' });
  });

  it('lists stored accounts with their type badges', async () => {
    seedAccounts();
    render(<AccountManager onSelectAccount={vi.fn()} onAddAccount={vi.fn()} />);

    expect(await screen.findByText('steve')).toBeTruthy();
    expect(screen.getByText('SteveMS')).toBeTruthy();
    expect(screen.getByText('oldy')).toBeTruthy();
    // Two usable HRPAuth-flavored badges (hrpauth + microsoft) and one legacy.
    expect(screen.getAllByText('HRPAuth')).toHaveLength(2);
    expect(screen.getByText('Legacy')).toBeTruthy();
  });

  it('shows the empty state when no accounts are stored', async () => {
    localStorage.setItem('accounts_schema', '3');
    render(<AccountManager onSelectAccount={vi.fn()} onAddAccount={vi.fn()} />);

    expect(await screen.findByText('No accounts')).toBeTruthy();
  });

  it('hydrates keychain tokens and marks last used on selection', async () => {
    seedAccounts();
    mockLoad.mockResolvedValue({
      success: true,
      data: { accessToken: 'at-1', clientToken: 'ct-1' },
      error: '',
    });
    const onSelectAccount = vi.fn();
    render(<AccountManager onSelectAccount={onSelectAccount} onAddAccount={vi.fn()} />);

    fireEvent.click(await screen.findByLabelText('Use account steve'));

    await waitFor(() => expect(onSelectAccount).toHaveBeenCalledTimes(1));
    expect(mockLoad).toHaveBeenCalledWith('a1');
    expect(onSelectAccount.mock.calls[0][0]).toMatchObject({
      id: 'a1',
      username: 'steve',
      accessToken: 'at-1',
      clientToken: 'ct-1',
    });
    // lastUsed moved to the selected account in the persisted metadata.
    const stored = JSON.parse(localStorage.getItem(ACCOUNTS_STORAGE_KEY) ?? '[]');
    expect(stored.find((a: { id: string }) => a.id === 'a1').lastUsed).toBe(true);
    expect(stored.find((a: { id: string }) => a.id === 'a2').lastUsed ?? false).toBe(false);
  });

  it('asks for a re-login when the keychain has no tokens for the account', async () => {
    seedAccounts();
    mockLoad.mockResolvedValue({ success: false, data: null, error: 'not found' });
    const onSelectAccount = vi.fn();
    render(<AccountManager onSelectAccount={onSelectAccount} onAddAccount={vi.fn()} />);

    fireEvent.click(await screen.findByLabelText('Use account steve'));

    expect(await screen.findByText('This account needs to sign in again')).toBeTruthy();
    expect(onSelectAccount).not.toHaveBeenCalled();
  });

  it('refuses to select a legacy account', async () => {
    seedAccounts();
    const onSelectAccount = vi.fn();
    render(<AccountManager onSelectAccount={onSelectAccount} onAddAccount={vi.fn()} />);

    fireEvent.click(await screen.findByLabelText('Use account oldy'));

    expect(await screen.findByText('Sign in again with HRPAuth')).toBeTruthy();
    expect(onSelectAccount).not.toHaveBeenCalled();
    expect(mockLoad).not.toHaveBeenCalled();
  });

  it('deletes the metadata and the keychain entry after confirmation', async () => {
    seedAccounts();
    render(<AccountManager onSelectAccount={vi.fn()} onAddAccount={vi.fn()} />);

    const deleteButtons = await screen.findAllByLabelText('Delete');
    fireEvent.click(deleteButtons[0]); // steve

    // The confirmation dialog names the account; confirm inside it.
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByText(/steve/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));

    await waitFor(() => expect(mockDelete).toHaveBeenCalledWith('a1'));
    await waitFor(() => {
      const stored = JSON.parse(localStorage.getItem(ACCOUNTS_STORAGE_KEY) ?? '[]');
      expect(stored.map((a: { id: string }) => a.id)).toEqual(['a2', 'a3']);
    });
  });
});
