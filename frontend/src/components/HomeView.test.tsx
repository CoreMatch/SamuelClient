import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import HomeView from './HomeView';
import { StoredAccount } from '../accountStorage';
import { InstalledVersion } from '../types';
import {
  GetJavaInstallations,
  GetInstalledVersions,
  GetFabricVersions,
  GetForgeVersions,
  InstallFabricLoader,
  InstallForgeLoader,
  LaunchGame,
} from '../../wailsjs/go/main/App';

// All backend bindings are replaced with test doubles; HomeView must never
// reach a real backend. The runtime event subscription is stubbed out too.
vi.mock('../../wailsjs/go/main/App', () => ({
  GetJavaInstallations: vi.fn(),
  GetInstalledVersions: vi.fn(),
  GetFabricVersions: vi.fn(),
  GetForgeVersions: vi.fn(),
  InstallFabricLoader: vi.fn(),
  InstallForgeLoader: vi.fn(),
  LaunchGame: vi.fn(),
}));
vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
}));

const mockGetInstalledVersions = vi.mocked(GetInstalledVersions);
const mockGetJavaInstallations = vi.mocked(GetJavaInstallations);
const mockGetFabricVersions = vi.mocked(GetFabricVersions);
const mockGetForgeVersions = vi.mocked(GetForgeVersions);
const mockInstallFabricLoader = vi.mocked(InstallFabricLoader);
const mockInstallForgeLoader = vi.mocked(InstallForgeLoader);
const mockLaunchGame = vi.mocked(LaunchGame);

const account: StoredAccount = {
  id: 'a1',
  username: 'Steve',
  uuid: 'uuid-1',
  accessToken: 'access-1',
  clientToken: 'client-1',
  authType: 'hrpauth',
};

const version: InstalledVersion = { id: '1.99', type: 'release', installed: true };

function emptyLists() {
  mockGetInstalledVersions.mockResolvedValue({ success: true, data: { versions: [] }, error: '' });
  mockGetJavaInstallations.mockResolvedValue({ success: true, data: [], error: '' });
  mockGetFabricVersions.mockResolvedValue({ success: true, data: [], error: '' });
  mockGetForgeVersions.mockResolvedValue({ success: true, data: [], error: '' });
}

// Opens the nth Radix select (combobox role) via keyboard and picks the
// option by accessible name.
async function selectOption(index: number, name: string) {
  fireEvent.keyDown(screen.getAllByRole('combobox')[index], { key: 'Enter' });
  const option = await screen.findByRole('option', { name });
  fireEvent.pointerUp(option);
  fireEvent.click(option);
}

describe('HomeView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
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

  it('installs a fabric loader through the binding and refreshes versions', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [{ id: '1.99', type: 'release', has_json: true, has_jar: true }] },
      error: '',
    });
    mockGetFabricVersions.mockResolvedValue({
      success: true,
      data: [{ loader: { version: '0.15.0' } }],
      error: '',
    });
    mockInstallFabricLoader.mockResolvedValue({
      success: true,
      data: { version_id: 'fabric-loader-0.15.0-1.99' },
      error: '',
    });

    render(
      <HomeView
        selectedVersion={null}
        onVersionSelect={vi.fn()}
        currentAccount={account}
        onOpenSettings={vi.fn()}
      />,
    );

    await selectOption(0, '1.99');

    // The loader dropdown offers the fabric loader once fetchLoaders resolves;
    // picking it installs through the binding.
    await waitFor(() => expect(mockGetFabricVersions).toHaveBeenCalled());
    await selectOption(1, 'fabric 0.15.0');

    await waitFor(() =>
      expect(mockInstallFabricLoader).toHaveBeenCalledWith('1.99', '0.15.0', ''),
    );
    expect(mockInstallForgeLoader).not.toHaveBeenCalled();
    // Success refreshes the installed version list (initial load + reload).
    await waitFor(() => expect(mockGetInstalledVersions.mock.calls.length).toBe(2));
  });

  it('surfaces the error when a forge loader install fails', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [{ id: '1.99', type: 'release', has_json: true, has_jar: true }] },
      error: '',
    });
    mockGetForgeVersions.mockResolvedValue({
      success: true,
      data: [{ version: '47.2.0' }],
      error: '',
    });
    mockInstallForgeLoader.mockResolvedValue({
      success: false,
      data: null,
      error: 'forge installer failed',
    });

    render(
      <HomeView
        selectedVersion={null}
        onVersionSelect={vi.fn()}
        currentAccount={account}
        onOpenSettings={vi.fn()}
      />,
    );

    await selectOption(0, '1.99');

    await waitFor(() => expect(mockGetForgeVersions).toHaveBeenCalled());
    await selectOption(1, 'forge 47.2.0');

    expect(await screen.findByText('forge installer failed')).toBeTruthy();
    expect(mockInstallForgeLoader).toHaveBeenCalledWith('1.99', '47.2.0', '');
  });
});
