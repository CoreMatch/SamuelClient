import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import ProfileView from './ProfileView';
import type { StoredAccount } from '../accountStorage';
import {
  DeleteProfileTexture,
  GetProfileTextures,
  UploadProfileTexture,
} from '../../wailsjs/go/main/App';

vi.mock('../../wailsjs/go/main/App', () => ({
  DeleteProfileTexture: vi.fn(),
  GetProfileTextures: vi.fn(),
  UploadProfileTexture: vi.fn(),
}));

const mockGetTextures = vi.mocked(GetProfileTextures);
const mockDeleteTexture = vi.mocked(DeleteProfileTexture);
const account: StoredAccount = {
  id: 'account-1',
  username: 'Steve',
  uuid: '0123456789abcdef0123456789abcdef',
  authType: 'hrpauth',
  accessToken: 'session-token',
  clientToken: 'client-token',
};
const textureResponse = {
  success: true,
  error: '',
  data: {
    profiles: [{
      id: account.uuid,
      name: account.username,
      textures: {
        skin: { url: 'https://ha.mcnb.dev/textures/skin-hash' },
      },
    }],
  },
};

describe('ProfileView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetTextures.mockResolvedValue(textureResponse);
    mockDeleteTexture.mockResolvedValue({ success: true, data: null, error: '' });
    vi.mocked(UploadProfileTexture).mockResolvedValue({ success: true, data: null, error: '' });
  });

  it('shows the signed-in player identity and current texture state', async () => {
    render(<ProfileView account={account} onSignIn={vi.fn()} onSignOut={vi.fn()} />);

    expect(await screen.findByRole('heading', { name: 'Steve' })).toBeTruthy();
    expect(screen.getByText(account.uuid)).toBeTruthy();
    expect(screen.getByText('Skin')).toBeTruthy();
    expect(screen.getByText('Cape')).toBeTruthy();
    expect(screen.getByText('Active')).toBeTruthy();
    expect(screen.getByText('Not set')).toBeTruthy();
    expect(mockGetTextures).toHaveBeenCalledWith('session-token');
  });

  it('uses the exchanged Yggdrasil session for Microsoft sign-ins, not OAuth credentials', async () => {
    const microsoftAccount = {
      ...account,
      authType: 'microsoft' as const,
      accessToken: 'yggdrasil-session-from-microsoft-exchange',
      msaRefreshToken: 'msa-oauth-refresh-token',
    };
    render(<ProfileView account={microsoftAccount} onSignIn={vi.fn()} onSignOut={vi.fn()} />);

    await screen.findByRole('heading', { name: 'Steve' });

    expect(mockGetTextures).toHaveBeenCalledWith('yggdrasil-session-from-microsoft-exchange');
    expect(mockGetTextures).not.toHaveBeenCalledWith('msa-oauth-refresh-token');
  });

  it('confirms and removes the active cape through Yggdrasil', async () => {
    const capeResponse = {
      ...textureResponse,
      data: {
        profiles: [{
          ...textureResponse.data.profiles[0],
          textures: { cape: { url: 'https://ha.mcnb.dev/textures/cape-hash' } },
        }],
      },
    };
    mockGetTextures.mockResolvedValue(capeResponse);
    render(<ProfileView account={account} onSignIn={vi.fn()} onSignOut={vi.fn()} />);

    fireEvent.click(await screen.findByRole('button', { name: 'Remove' }));
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Remove' }));

    await waitFor(() => {
      expect(mockDeleteTexture).toHaveBeenCalledWith(account.accessToken, account.uuid, 'cape');
    });
    expect(await screen.findByText('Cape removed successfully.')).toBeTruthy();
  });

  it('offers sign-in when there is no active player', () => {
    const onSignIn = vi.fn();
    render(<ProfileView account={null} onSignIn={onSignIn} onSignOut={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(onSignIn).toHaveBeenCalledOnce();
    expect(mockGetTextures).not.toHaveBeenCalled();
  });
});
