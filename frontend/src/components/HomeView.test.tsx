import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import HomeView from './HomeView';
import { StoredAccount } from '../accountStorage';
import { InstalledVersion } from '../types';
import {
  GetJavaInstallations,
  GetInstalledVersions,
  LaunchGame,
} from '../../wailsjs/go/main/App';

// All backend bindings are replaced with test doubles; HomeView must never
// reach a real backend. The runtime event subscription is stubbed out too.
vi.mock('../../wailsjs/go/main/App', () => ({
  GetJavaInstallations: vi.fn(),
  GetInstalledVersions: vi.fn(),
  GetFabricVersions: vi.fn(),
  GetForgeVersions: vi.fn(),
  LaunchGame: vi.fn(),
}));
vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
}));

const mockGetInstalledVersions = vi.mocked(GetInstalledVersions);
const mockGetJavaInstallations = vi.mocked(GetJavaInstallations);
const mockLaunchGame = vi.mocked(LaunchGame);

const account: StoredAccount = {
  id: 'a1',
  username: 'Steve',
  uuid: 'uuid-1',
  accessToken: 'access-1',
  clientToken: 'client-1',
  authType: 'offline',
};

const version: InstalledVersion = { id: '1.99', type: 'release', installed: true };

function emptyLists() {
  mockGetInstalledVersions.mockResolvedValue({ success: true, data: { versions: [] }, error: '' });
  mockGetJavaInstallations.mockResolvedValue({ success: true, data: [], error: '' });
}

describe('HomeView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    emptyLists();
  });

  it('disables Play when no version or account is selected', async () => {
    render(
      <HomeView
        selectedVersion={null}
        onVersionSelect={vi.fn()}
        currentAccount={null}
        onOpenSettings={vi.fn()}
      />,
    );

    const play = screen.getByRole('button', { name: 'Play' });
    expect(play.hasAttribute('disabled')).toBe(true);
    expect(await screen.findByText('No account selected')).toBeTruthy();
    expect(mockGetInstalledVersions).toHaveBeenCalledTimes(1);
  });

  it('launches the selected version with the active account', async () => {
    mockLaunchGame.mockResolvedValue({ success: true, data: { pid: 42 }, error: '' });

    render(
      <HomeView
        selectedVersion={version}
        onVersionSelect={vi.fn()}
        currentAccount={account}
        onOpenSettings={vi.fn()}
      />,
    );

    const play = await waitFor(() => {
      const btn = screen.getByRole('button', { name: 'Play' });
      expect(btn.hasAttribute('disabled')).toBe(false);
      return btn;
    });
    fireEvent.click(play);

    await waitFor(() => expect(mockLaunchGame).toHaveBeenCalledTimes(1));
    const request = mockLaunchGame.mock.calls[0][0];
    expect(request.version_id).toBe('1.99');
    expect(request.username).toBe('Steve');
    expect(request.uuid).toBe('uuid-1');
    expect(request.access_token).toBe('access-1');
    expect(request.client_token).toBe('client-1');
  });

  it('shows the backend error when launching fails', async () => {
    mockLaunchGame.mockResolvedValue({ success: false, data: null, error: 'java not found' });

    render(
      <HomeView
        selectedVersion={version}
        onVersionSelect={vi.fn()}
        currentAccount={account}
        onOpenSettings={vi.fn()}
      />,
    );

    fireEvent.click(await screen.findByRole('button', { name: 'Play' }));
    expect(await screen.findByText('java not found')).toBeTruthy();
  });
});
