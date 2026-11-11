import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import LoginView, { LoggedInAccount } from './LoginView';
import {
  HRPAuthLogin,
  MicrosoftBeginDeviceCode,
  MicrosoftCompleteDeviceCode,
} from '../../wailsjs/go/main/App';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';

// The Wails bindings are replaced with test doubles; LoginView must never
// reach a real backend.
vi.mock('../../wailsjs/go/main/App', () => ({
  HRPAuthLogin: vi.fn(),
  MicrosoftBeginDeviceCode: vi.fn(),
  MicrosoftCompleteDeviceCode: vi.fn(),
}));
vi.mock('../../wailsjs/runtime/runtime', () => ({
  BrowserOpenURL: vi.fn(),
}));

const mockHRPAuthLogin = vi.mocked(HRPAuthLogin);
const mockMSBegin = vi.mocked(MicrosoftBeginDeviceCode);
const mockMSComplete = vi.mocked(MicrosoftCompleteDeviceCode);
const mockOpenURL = vi.mocked(BrowserOpenURL);

const deviceCodeData = {
  device_code: 'device-1',
  user_code: 'ABCD-EFGH',
  verification_uri: 'http://127.0.0.1:8088/device',
  expires_in: 900,
  interval: 1,
};

describe('LoginView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('rejects login without credentials before calling the backend', async () => {
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

  it('offers no offline login path anymore (ADR-0008)', () => {
    render(<LoginView onLoginSuccess={vi.fn()} onSwitchToAccounts={vi.fn()} />);

    // No auth-type selector and no offline wording anywhere in the view.
    expect(screen.queryAllByRole('combobox')).toHaveLength(0);
    expect(screen.queryByText(/offline/i)).toBeNull();
    expect(screen.queryByText(/cracked/i)).toBeNull();
  });

  describe('Microsoft device-code login (ADR-0007)', () => {
    it('shows the device code and propagates the exchanged Microsoft account', async () => {
      let resolveComplete!: (value: unknown) => void;
      const completePromise = new Promise(resolve => {
        resolveComplete = resolve;
      });
      mockMSBegin.mockResolvedValue({ success: true, data: deviceCodeData, error: '' });
      mockMSComplete.mockReturnValue(completePromise as ReturnType<typeof MicrosoftCompleteDeviceCode>);
      const onLoginSuccess = vi.fn();
      render(<LoginView onLoginSuccess={onLoginSuccess} onSwitchToAccounts={vi.fn()} />);

      fireEvent.click(screen.getByRole('button', { name: 'Sign in with Microsoft' }));

      // The device-code stage renders the user code and the verification URL.
      expect(await screen.findByText('ABCD-EFGH')).toBeTruthy();
      expect(screen.getByText('http://127.0.0.1:8088/device')).toBeTruthy();
      expect(mockMSComplete).toHaveBeenCalledWith('device-1', 900, 1);

      fireEvent.click(screen.getByRole('button', { name: 'Open sign-in page' }));
      expect(mockOpenURL).toHaveBeenCalledWith('http://127.0.0.1:8088/device');

      resolveComplete({
        success: true,
        data: {
          accessToken: 'ms-access',
          clientToken: 'ms-client',
          msaRefreshToken: 'msa-refresh',
          uuid: 'uuid-ms',
          username: 'SteveMS',
          claimRequired: false,
        },
        error: '',
      });

      await waitFor(() => expect(onLoginSuccess).toHaveBeenCalledTimes(1));
      const account: LoggedInAccount = onLoginSuccess.mock.calls[0][0];
      expect(account.username).toBe('SteveMS');
      expect(account.uuid).toBe('uuid-ms');
      expect(account.accessToken).toBe('ms-access');
      expect(account.authType).toBe('microsoft');
      expect(account.msaRefreshToken).toBe('msa-refresh');
    });

    it('surfaces the error when the device-code start fails', async () => {
      mockMSBegin.mockResolvedValue({ success: false, data: null, error: 'devicecode status 500' });
      render(<LoginView onLoginSuccess={vi.fn()} onSwitchToAccounts={vi.fn()} />);

      fireEvent.click(screen.getByRole('button', { name: 'Sign in with Microsoft' }));

      expect(await screen.findByText('devicecode status 500')).toBeTruthy();
      expect(mockMSComplete).not.toHaveBeenCalled();
      // The credential form stays usable.
      expect(screen.getByRole('button', { name: 'Sign in with Microsoft' })).toBeTruthy();
    });

    it('surfaces the exchange error and returns to the form', async () => {
      mockMSBegin.mockResolvedValue({ success: true, data: deviceCodeData, error: '' });
      mockMSComplete.mockResolvedValue({ success: false, data: null, error: 'mc_token_invalid' });
      render(<LoginView onLoginSuccess={vi.fn()} onSwitchToAccounts={vi.fn()} />);

      fireEvent.click(screen.getByRole('button', { name: 'Sign in with Microsoft' }));

      // The exchange fails fast; the view lands back on the form with the error.
      expect(await screen.findByText('mc_token_invalid')).toBeTruthy();
      // Back on the form, the Microsoft button is offered again.
      expect(await screen.findByRole('button', { name: 'Sign in with Microsoft' })).toBeTruthy();
    });

    it('drops a late exchange result after cancel', async () => {
      let resolveComplete!: (value: unknown) => void;
      const completePromise = new Promise(resolve => {
        resolveComplete = resolve;
      });
      mockMSBegin.mockResolvedValue({ success: true, data: deviceCodeData, error: '' });
      mockMSComplete.mockReturnValue(completePromise as ReturnType<typeof MicrosoftCompleteDeviceCode>);
      const onLoginSuccess = vi.fn();
      render(<LoginView onLoginSuccess={onLoginSuccess} onSwitchToAccounts={vi.fn()} />);

      fireEvent.click(screen.getByRole('button', { name: 'Sign in with Microsoft' }));
      expect(await screen.findByText('ABCD-EFGH')).toBeTruthy();

      fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
      expect(await screen.findByRole('button', { name: 'Sign in with Microsoft' })).toBeTruthy();

      resolveComplete({
        success: true,
        data: {
          accessToken: 'late-access',
          clientToken: 'late-client',
          msaRefreshToken: 'late-refresh',
          uuid: 'uuid-late',
          username: 'Late',
        },
        error: '',
      });
      // Give the dropped promise a chance to flush.
      await new Promise(r => setTimeout(r, 0));
      expect(onLoginSuccess).not.toHaveBeenCalled();
    });
  });
});
