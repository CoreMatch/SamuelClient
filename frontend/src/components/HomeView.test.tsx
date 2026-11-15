import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
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
  authType: 'hrpauth',
};

const version: InstalledVersion = { id: '1.99', type: 'release', installed: true };

function emptyLists() {
  mockGetInstalledVersions.mockResolvedValue({ success: true, data: { versions: [] }, error: '' });
  mockGetJavaInstallations.mockResolvedValue({ success: true, data: [], error: '' });
}

// Opens the nth Radix select (combobox role) via keyboard and picks the
// option by accessible name.
async function selectOption(index: number, name: string) {
  fireEvent.keyDown(screen.getAllByRole('combobox')[index], { key: 'Enter' });
  const option = await screen.findByRole('option', { name });
  fireEvent.pointerUp(option);
  fireEvent.click(option);
}

function renderHome(selectedVersion: InstalledVersion | null = null) {
  return render(
    <HomeView
      selectedVersion={selectedVersion}
      onVersionSelect={vi.fn()}
      currentAccount={account}
      onOpenSettings={vi.fn()}
    />,
  );
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

    renderHome(version);

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

    renderHome(version);

    fireEvent.click(await screen.findByRole('button', { name: 'Play' }));
    expect(await screen.findByText('java not found')).toBeTruthy();
  });

  it('shows None for a vanilla version', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [{ id: '1.99', type: 'release', has_json: true, has_jar: true }] },
      error: '',
    });

    renderHome();
    await act(async () => {
      await selectOption(0, '1.99');
    });

    expect(screen.getByText('None')).toBeTruthy();
  });

  it('shows the installed Fabric loader for a fabric version', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: {
        versions: [
          { id: '1.99', type: 'release', has_json: true, has_jar: true },
          { id: 'fabric-loader-0.15.0-1.99', type: 'release', has_json: true, has_jar: true },
        ],
      },
      error: '',
    });

    renderHome();
    await act(async () => {
      await selectOption(0, 'fabric-loader-0.15.0-1.99');
    });

    expect(screen.getByText('Fabric 0.15.0')).toBeTruthy();
  });

  it('shows the installed Forge loader for a forge version', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [{ id: '1.99-forge49.0.3', type: 'release', has_json: true, has_jar: true }] },
      error: '',
    });

    renderHome();
    await act(async () => {
      await selectOption(0, '1.99-forge49.0.3');
    });

    expect(screen.getByText('Forge 49.0.3')).toBeTruthy();
  });

  it('launches with the loader derived from the selected version', async () => {
    mockLaunchGame.mockResolvedValue({ success: true, data: { pid: 7 }, error: '' });
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [{ id: 'fabric-loader-0.15.0-1.99', type: 'release', has_json: true, has_jar: true }] },
      error: '',
    });

    renderHome();
    await act(async () => {
      await selectOption(0, 'fabric-loader-0.15.0-1.99');
    });

    fireEvent.click(await screen.findByRole('button', { name: 'Play' }));
    await waitFor(() => expect(mockLaunchGame).toHaveBeenCalledTimes(1));
    const request = mockLaunchGame.mock.calls[0][0];
    expect(request.version_id).toBe('fabric-loader-0.15.0-1.99');
    expect(request.loader_type).toBe('fabric');
    expect(request.loader_version).toBe('0.15.0');
  });
});