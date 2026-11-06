import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import LoginView, { LoggedInAccount } from './LoginView';
import { HRPAuthLogin } from '../../wailsjs/go/main/App';

// The Wails bindings are replaced with test doubles; LoginView must never
// reach a real backend.
vi.mock('../../wailsjs/go/main/App', () => ({
  HRPAuthLogin: vi.fn(),
}));

const mockHRPAuthLogin = vi.mocked(HRPAuthLogin);

// Opens the auth-type select (first combobox) and picks an option by name.
async function selectAuthType(name: string) {
  fireEvent.keyDown(screen.getAllByRole('combobox')[0], { key: 'Enter' });
  const option = await screen.findByRole('option', { name });
  fireEvent.pointerUp(option);
  fireEvent.click(option);
}

describe('LoginView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('rejects HRPAuth login without credentials before calling the backend', async () => {
    const onLoginSuccess = vi.fn();
    render(<LoginView onLoginSuccess={onLoginSuccess} onSwitchToAccounts={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    expect(await screen.findByText('Username and password are required')).toBeTruthy();
    expect(mockHRPAuthLogin).not.toHaveBeenCalled();
    expect(onLoginSuccess).not.toHaveBeenCalled();
  });

  it('propagates the account on successful HRPAuth login', async () => {
    mockHRPAuthLogin.mockResolvedValue({
      success: true,
      data: {
        accessToken: 'access-1',
        clientToken: 'client-1',
        selectedProfile: { id: 'uuid-1', name: 'Steve' },
      },
      error: '',
    });
    const onLoginSuccess = vi.fn();
    render(<LoginView onLoginSuccess={onLoginSuccess} onSwitchToAccounts={vi.fn()} />);

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'steve' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'secret' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() => expect(onLoginSuccess).toHaveBeenCalledTimes(1));
    const account: LoggedInAccount = onLoginSuccess.mock.calls[0][0];
    expect(account.username).toBe('Steve');
    expect(account.uuid).toBe('uuid-1');
    expect(account.accessToken).toBe('access-1');
    expect(account.authType).toBe('hrpauth');
    // The endpoint is built in — no server argument travels through the
    // binding anymore.
    expect(mockHRPAuthLogin).toHaveBeenCalledWith('steve', 'secret', '');
  });

  it('surfaces the backend error on failed HRPAuth login', async () => {
    mockHRPAuthLogin.mockResolvedValue({
      success: false,
      data: null,
      error: 'Invalid credentials',
    });
    render(<LoginView onLoginSuccess={vi.fn()} onSwitchToAccounts={vi.fn()} />);

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'steve' } });
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'wrong' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    expect(await screen.findByText('Invalid credentials')).toBeTruthy();
  });

  it('logs in offline with just a username', async () => {
    const onLoginSuccess = vi.fn();
    render(<LoginView onLoginSuccess={onLoginSuccess} onSwitchToAccounts={vi.fn()} />);

    // Switch the auth type select to "Offline (Cracked)".
    await selectAuthType('Offline (Cracked)');

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'offline-steve' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() => expect(onLoginSuccess).toHaveBeenCalledTimes(1));
    const account: LoggedInAccount = onLoginSuccess.mock.calls[0][0];
    expect(account.authType).toBe('offline');
    expect(account.username).toBe('offline-steve');
    expect(account.uuid).not.toBe('');
    expect(mockHRPAuthLogin).not.toHaveBeenCalled();
  });

  it('requires a username for offline login', async () => {
    const onLoginSuccess = vi.fn();
    render(<LoginView onLoginSuccess={onLoginSuccess} onSwitchToAccounts={vi.fn()} />);

    await selectAuthType('Offline (Cracked)');

    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    expect(await screen.findByText('Username is required')).toBeTruthy();
    expect(onLoginSuccess).not.toHaveBeenCalled();
  });
});
