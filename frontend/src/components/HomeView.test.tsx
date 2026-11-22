import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import HomeView from './HomeView';
import { StoredAccount } from '../accountStorage';
import { InstalledVersion } from '../types';
import { saveVersionNickname } from '../versionNicknames';
import {
  GetJavaInstallations,
  GetInstalledVersions,
  GetLauncherConfig,
  UpdateLauncherConfig,
  LaunchGame,
  GetFabricVersions,
  GetForgeVersions,
  InstallFabricLoader,
  InstallForgeLoader,
  RemoveLoaderProfile,
} from '../../wailsjs/go/main/App';

// All backend bindings are replaced with test doubles; HomeView must never
// reach a real backend. The runtime event subscription is stubbed out too.
vi.mock('../../wailsjs/go/main/App', () => ({
  GetJavaInstallations: vi.fn(),
  GetInstalledVersions: vi.fn(),
  GetLauncherConfig: vi.fn(),
  UpdateLauncherConfig: vi.fn(),
  LaunchGame: vi.fn(),
  GetFabricVersions: vi.fn(),
  GetForgeVersions: vi.fn(),
  InstallFabricLoader: vi.fn(),
  InstallForgeLoader: vi.fn(),
  RemoveLoaderProfile: vi.fn(),
}));
vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
}));

const mockGetInstalledVersions = vi.mocked(GetInstalledVersions);
const mockGetJavaInstallations = vi.mocked(GetJavaInstallations);
const mockGetLauncherConfig = vi.mocked(GetLauncherConfig);
const mockUpdateLauncherConfig = vi.mocked(UpdateLauncherConfig);
const mockLaunchGame = vi.mocked(LaunchGame);
const mockGetFabricVersions = vi.mocked(GetFabricVersions);
const mockGetForgeVersions = vi.mocked(GetForgeVersions);
const mockInstallFabricLoader = vi.mocked(InstallFabricLoader);
const mockInstallForgeLoader = vi.mocked(InstallForgeLoader);
const mockRemoveLoaderProfile = vi.mocked(RemoveLoaderProfile);

const account: StoredAccount = {
  id: 'a1',
  username: 'Steve',
  uuid: 'uuid-1',
  accessToken: 'access-1',
  clientToken: 'client-1',
  authType: 'hrpauth',
};

const version: InstalledVersion = { id: '1.99', type: 'release', installed: true };

const defaultConfig = {
  java_path: '',
  max_memory: 4096,
  min_memory: 1024,
  java_args: '',
  game_width: 854,
  game_height: 480,
  game_dir: '',
  java_dir: '',
  bmclapi_mirrors: '',
  authlib_jar: '',
};

const javaInstallations = [
  { id: 1, path: '/usr/lib/jvm/8', friendly_name: 'Java 8', version: '1.8', is_default: true },
  { id: 2, path: '/usr/lib/jvm/17', friendly_name: 'Java 17', version: '17', is_default: false },
];

function emptyLists() {
  mockGetInstalledVersions.mockResolvedValue({ success: true, data: { versions: [] }, error: '' });
  mockGetJavaInstallations.mockResolvedValue({ success: true, data: [], error: '' });
  mockGetLauncherConfig.mockResolvedValue({ success: true, data: defaultConfig, error: '' });
  mockUpdateLauncherConfig.mockResolvedValue({ success: true, data: defaultConfig, error: '' });
  mockGetFabricVersions.mockResolvedValue({ success: true, data: [], error: '' });
  mockGetForgeVersions.mockResolvedValue({ success: true, data: [], error: '' });
  mockInstallFabricLoader.mockResolvedValue({ success: true, data: { version_id: 'fabric-loader-0.15.0-1.99' }, error: '' });
  mockInstallForgeLoader.mockResolvedValue({ success: true, data: { version_id: '1.99-forge-47.2.0' }, error: '' });
  mockRemoveLoaderProfile.mockResolvedValue({ success: true, data: {}, error: '' });
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

  it('shows the selected version nickname and launches with the original ID and active account', async () => {
    const installedVersion = { ...version, gameDir: '/games/main' };
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [{ id: '1.99', type: 'release', has_json: true, has_jar: true, game_dir: '/games/main' }] },
      error: '',
    });
    mockLaunchGame.mockResolvedValue({ success: true, data: { pid: 42 }, error: '' });
    saveVersionNickname(installedVersion, 'Main world');

    renderHome(installedVersion);

    await waitFor(() => {
      expect(screen.getByRole('combobox', { name: 'Minecraft Version' }).textContent).toBe('Main world');
    });
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
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [{ id: '1.99', type: 'release', has_json: true, has_jar: true }] },
      error: '',
    });
    mockLaunchGame.mockResolvedValue({ success: false, data: null, error: 'java not found' });

    renderHome(version);

    fireEvent.click(await screen.findByRole('button', { name: 'Play' }));
    expect(await screen.findByText('java not found')).toBeTruthy();
  });

  it('launches the selected copy when the same version is installed in two folders', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: ['/games/default', '/games/second'].map(game_dir => ({
        id: '1.99', type: 'release', has_json: true, has_jar: true, game_dir,
      })) },
      error: '',
    });
    mockLaunchGame.mockResolvedValue({ success: true, data: { pid: 42 }, error: '' });
    saveVersionNickname({ ...version, gameDir: '/games/second' }, 'Second copy');
    renderHome();

    await act(async () => {
      await selectOption(0, 'Second copy');
    });
    expect(screen.getByRole('combobox', { name: 'Minecraft Version' }).textContent).toBe('Second copy');
    fireEvent.click(screen.getByRole('button', { name: 'Play' }));

    await waitFor(() => expect(mockLaunchGame).toHaveBeenCalledWith(expect.objectContaining({
      version_id: '1.99',
      game_dir: '/games/second',
    })));
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

  it('adds Fabric to an already installed Minecraft version', async () => {
    const gameDir = '/games/installed';
    mockGetInstalledVersions
      .mockResolvedValueOnce({
        success: true,
        data: { versions: [{ id: '1.99', type: 'release', has_json: true, has_jar: true, game_dir: gameDir }] },
        error: '',
      })
      .mockResolvedValueOnce({
        success: true,
        data: {
          versions: [
            { id: '1.99', type: 'release', has_json: true, has_jar: true, game_dir: gameDir },
            { id: 'fabric-loader-0.15.0-1.99', type: 'release', has_json: true, has_jar: true, game_dir: gameDir, inherits_from: '1.99' },
          ],
        },
        error: '',
      });
    mockGetFabricVersions.mockResolvedValue({
      success: true,
      data: [{ loader: { version: '0.15.0' } }],
      error: '',
    });

    renderHome();
    await act(async () => {
      await selectOption(0, '1.99');
    });
    fireEvent.click(await screen.findByRole('button', { name: 'Add loader' }));

    await waitFor(() => {
      expect(mockInstallFabricLoader).toHaveBeenCalledWith('1.99', '0.15.0', gameDir);
      expect(screen.getByText('Fabric 0.15.0')).toBeTruthy();
    });
  });

  it('removes a selected loader profile while keeping its base game selected', async () => {
    const gameDir = '/games/installed';
    mockGetInstalledVersions
      .mockResolvedValueOnce({
        success: true,
        data: {
          versions: [
            { id: '1.99', type: 'release', has_json: true, has_jar: true, game_dir: gameDir },
            { id: 'fabric-loader-0.15.0-1.99', type: 'release', has_json: true, has_jar: true, game_dir: gameDir, inherits_from: '1.99' },
          ],
        },
        error: '',
      })
      .mockResolvedValueOnce({
        success: true,
        data: { versions: [{ id: '1.99', type: 'release', has_json: true, has_jar: true, game_dir: gameDir }] },
        error: '',
      });
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);

    renderHome();
    await act(async () => {
      await selectOption(0, 'fabric-loader-0.15.0-1.99');
    });
    fireEvent.click(screen.getByRole('button', { name: 'Remove this loader' }));

    await waitFor(() => {
      expect(mockRemoveLoaderProfile).toHaveBeenCalledWith('fabric-loader-0.15.0-1.99', gameDir);
      expect(screen.getByText('None')).toBeTruthy();
    });
    confirm.mockRestore();
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

  it('loads launch settings from the backend', async () => {
    mockGetLauncherConfig.mockResolvedValue({
      success: true,
      data: { ...defaultConfig, max_memory: 8192 },
      error: '',
    });

    renderHome();

    fireEvent.click(screen.getByRole('button', { name: 'Launch Settings' }));
    expect(await screen.findByText('8192 MB')).toBeTruthy();
  });

  it('keeps Play launch settings temporary and links to permanent Settings', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [{ id: '1.99', type: 'release', has_json: true, has_jar: true }] },
      error: '',
    });
    mockLaunchGame.mockResolvedValue({ success: true, data: { pid: 42 }, error: '' });
    const onOpenSettings = vi.fn();
    render(
      <HomeView
        selectedVersion={version}
        onVersionSelect={vi.fn()}
        currentAccount={account}
        onOpenSettings={onOpenSettings}
      />,
    );

    const settingsToggle = screen.getByRole('button', { name: 'Launch Settings' });
    expect(settingsToggle.getAttribute('aria-expanded')).toBe('false');
    expect(screen.getByText(/Temporary launch settings/).closest('#play-launch-settings')?.hasAttribute('hidden')).toBe(true);
    fireEvent.click(settingsToggle);
    expect(settingsToggle.getAttribute('aria-expanded')).toBe('true');
    await screen.findByText('4096 MB');

    fireEvent.change(screen.getByLabelText('Width'), { target: { value: '1280' } });
    fireEvent.click(screen.getByRole('button', { name: 'Open Settings' }));
    expect(onOpenSettings).toHaveBeenCalledOnce();
    expect(mockUpdateLauncherConfig).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Play' }));
    await waitFor(() => expect(mockLaunchGame).toHaveBeenCalledOnce());
    expect(mockLaunchGame.mock.calls[0][0].game_width).toBe(1280);
  });

  it('adopts legacy localStorage settings once', async () => {
    localStorage.setItem('launcher_settings', JSON.stringify({ maxMemory: 2048 }));

    renderHome();

    fireEvent.click(screen.getByRole('button', { name: 'Launch Settings' }));
    expect(await screen.findByText('2048 MB')).toBeTruthy();
    await waitFor(() => expect(mockUpdateLauncherConfig).toHaveBeenCalledTimes(1));
    expect(mockUpdateLauncherConfig.mock.calls[0][0].max_memory).toBe(2048);
    expect(localStorage.getItem('launcher_settings')).toBeNull();
  });

  it('remembers the selected Java installation across restarts', async () => {
    mockGetJavaInstallations.mockResolvedValue({ success: true, data: javaInstallations, error: '' });

    const { unmount } = renderHome();
    await screen.findByText('Java 8 (1.8)');
    await act(async () => {
      await selectOption(1, 'Java 17 (17)');
    });
    expect(localStorage.getItem('selected_java')).toBe('2');
    unmount();

    renderHome();
    expect(await screen.findByText('Java 17 (17)')).toBeTruthy();
  });

  it('launches with the selected Java path', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [{ id: '1.99', type: 'release', has_json: true, has_jar: true }] },
      error: '',
    });
    mockGetJavaInstallations.mockResolvedValue({ success: true, data: javaInstallations, error: '' });
    mockLaunchGame.mockResolvedValue({ success: true, data: { pid: 9 }, error: '' });

    renderHome(version);
    await screen.findByText('Java 8 (1.8)');
    await act(async () => {
      await selectOption(3, 'Java 17 (17)');
    });

    fireEvent.click(await screen.findByRole('button', { name: 'Play' }));
    await waitFor(() => expect(mockLaunchGame).toHaveBeenCalledTimes(1));
    expect(mockLaunchGame.mock.calls[0][0].java_path).toBe('/usr/lib/jvm/17');
  });

  it('clears a restored version that is no longer installed', async () => {
    const onVersionSelect = vi.fn();
    render(
      <HomeView
        selectedVersion={{ id: 'gone', type: 'release', installed: true }}
        onVersionSelect={onVersionSelect}
        currentAccount={account}
        onOpenSettings={vi.fn()}
      />,
    );

    await waitFor(() => expect(onVersionSelect).toHaveBeenCalledWith(null));
  });
});
