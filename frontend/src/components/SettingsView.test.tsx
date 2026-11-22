import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import SettingsView from './SettingsView';
import { TooltipProvider } from '../ui/tooltip';
import {
  GetLauncherConfig,
  GetJavaInstallations,
  UpdateLauncherConfig,
  AddJavaInstallation,
  UpdateJavaInstallation,
  DeleteJavaInstallation,
  SetDefaultJavaInstallation,
  GetSupportedJavaRuntimeVersions,
  GetDefaultJavaRuntimePath,
  InstallJavaRuntime,
  GetGameDirStatus,
  GetDownloadSettings,
  SetDownloadSettings,
  SetGameDir,
  AddGameDir,
  RemoveGameDir,
  ScanGameDirs,
  AddMod,
  DeleteMod,
  ListMods,
  SetModEnabled,
} from '../../wailsjs/go/main/App';

// The Wails bindings are replaced with test doubles; the settings view must
// never reach a real backend.
vi.mock('../../wailsjs/go/main/App', () => ({
  GetLauncherConfig: vi.fn(),
  GetJavaInstallations: vi.fn(),
  UpdateLauncherConfig: vi.fn(),
  AddJavaInstallation: vi.fn(),
  UpdateJavaInstallation: vi.fn(),
  DeleteJavaInstallation: vi.fn(),
  SetDefaultJavaInstallation: vi.fn(),
  GetSupportedJavaRuntimeVersions: vi.fn(),
  GetDefaultJavaRuntimePath: vi.fn(),
  InstallJavaRuntime: vi.fn(),
  CancelInstall: vi.fn(),
  GetGameDirStatus: vi.fn(),
  GetDownloadSettings: vi.fn(),
  SetDownloadSettings: vi.fn(),
  SetGameDir: vi.fn(),
  AddGameDir: vi.fn(),
  RemoveGameDir: vi.fn(),
  ScanGameDirs: vi.fn(),
  AddMod: vi.fn(),
  DeleteMod: vi.fn(),
  ListMods: vi.fn(),
  SetModEnabled: vi.fn(),
}));

vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

const mockGetConfig = vi.mocked(GetLauncherConfig);
const mockGetJava = vi.mocked(GetJavaInstallations);
const mockUpdateConfig = vi.mocked(UpdateLauncherConfig);
const mockAddJava = vi.mocked(AddJavaInstallation);
const mockUpdateJava = vi.mocked(UpdateJavaInstallation);
const mockDeleteJava = vi.mocked(DeleteJavaInstallation);
const mockSetDefault = vi.mocked(SetDefaultJavaInstallation);
const mockGetJavaRuntimeVersions = vi.mocked(GetSupportedJavaRuntimeVersions);
const mockGetJavaRuntimePath = vi.mocked(GetDefaultJavaRuntimePath);
const mockInstallJavaRuntime = vi.mocked(InstallJavaRuntime);
const mockGetGameDirStatus = vi.mocked(GetGameDirStatus);
const mockSetGameDir = vi.mocked(SetGameDir);
const mockAddGameDir = vi.mocked(AddGameDir);
const mockRemoveGameDir = vi.mocked(RemoveGameDir);
const mockScanGameDirs = vi.mocked(ScanGameDirs);

const launcherConfig = {
  java_path: '/opt/java/bin/java',
  game_dir: '/home/player/.minecraft',
  max_memory: 4096,
  min_memory: 1024,
  game_width: 854,
  game_height: 480,
  java_args: '',
};

const javaRows = [
  {
    id: 1,
    path: '/opt/java17/bin/java',
    friendly_name: 'JDK 17',
    version: '17.0.9',
    is_default: true,
    created_at: '',
    updated_at: '',
  },
  {
    id: 2,
    path: '/opt/java21/bin/java',
    friendly_name: 'JDK 21',
    version: '21.0.2',
    is_default: false,
    created_at: '',
    updated_at: '',
  },
];

function ok(data: unknown) {
  return { success: true, data, error: '' };
}

const builtIn = { path: '/home/player/.minecraft', source: 'auto', exists: true };
const custom = { path: '/games/mc/.minecraft', source: 'custom', exists: true };

// The app shell wraps every view in the tooltip provider (App.tsx); the
// isolated view needs the same wrapper.
function renderView() {
  return render(
    <TooltipProvider>
      <SettingsView />
    </TooltipProvider>,
  );
}

describe('SettingsView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetConfig.mockResolvedValue(ok(launcherConfig));
    mockGetJava.mockResolvedValue(ok(javaRows));
    mockSetDefault.mockResolvedValue(ok(javaRows));
    mockGetJavaRuntimeVersions.mockResolvedValue(ok([8, 11, 17, 21, 25]));
    mockGetJavaRuntimePath.mockImplementation(async major => ok(`/launcher/jre/${major}`));
    mockInstallJavaRuntime.mockResolvedValue(ok({}));
    mockDeleteJava.mockResolvedValue(ok(javaRows));
    mockAddJava.mockResolvedValue(ok(javaRows));
    mockUpdateJava.mockResolvedValue(ok(javaRows));
    mockUpdateConfig.mockResolvedValue(ok({ ...launcherConfig, max_memory: 6144 }));
    mockGetGameDirStatus.mockResolvedValue(ok({ ...builtIn, dirs: [builtIn] }));
    mockAddGameDir.mockResolvedValue(ok({ ...builtIn, dirs: [builtIn, custom] }));
    mockSetGameDir.mockResolvedValue(ok({ ...custom, dirs: [builtIn, custom] }));
    mockRemoveGameDir.mockResolvedValue(ok({ ...builtIn, dirs: [builtIn] }));
    mockScanGameDirs.mockResolvedValue(ok({ dirs: [] }));
    vi.mocked(GetDownloadSettings).mockResolvedValue(
      ok({ source: 'bmclapi', custom_shape: 'bmclapi', custom_url: '', concurrency: 8 }),
    );
  });

  it('imports, disables, restores, and deletes a mod from the selected game folder', async () => {
    mockGetGameDirStatus.mockResolvedValue(ok({ ...custom, dirs: [builtIn, custom] }));
    const active = { name: 'example.jar', enabled: true };
    const disabled = { name: 'example.jar.bak', enabled: false };
    vi.mocked(ListMods)
      .mockResolvedValueOnce(ok([]))
      .mockResolvedValueOnce(ok([active]))
      .mockResolvedValueOnce(ok([disabled]))
      .mockResolvedValueOnce(ok([active]))
      .mockResolvedValueOnce(ok([]));
    vi.mocked(AddMod).mockResolvedValue(ok(true));
    vi.mocked(SetModEnabled).mockResolvedValue(ok(null));
    vi.mocked(DeleteMod).mockResolvedValue(ok(null));

    renderView();
    await screen.findByText(custom.path);
    fireEvent.click(screen.getByRole('button', { name: 'Manage Mods' }));
    await screen.findByText('No mods in this game folder.');
    expect(ListMods).toHaveBeenCalledWith(custom.path);

    fireEvent.click(screen.getByRole('button', { name: 'Add Mod' }));
    expect(AddMod).toHaveBeenCalledWith(custom.path);
    fireEvent.click(await screen.findByRole('button', { name: 'Disable example.jar' }));
    expect(SetModEnabled).toHaveBeenCalledWith(custom.path, active.name, false);
    fireEvent.click(await screen.findByRole('button', { name: 'Enable example.jar.bak' }));
    expect(SetModEnabled).toHaveBeenLastCalledWith(custom.path, disabled.name, true);

    fireEvent.click(await screen.findByRole('button', { name: 'Delete example.jar' }));
    expect(DeleteMod).not.toHaveBeenCalled();
    const confirmation = screen.getByRole('alertdialog', { name: 'Delete Mod' });
    fireEvent.click(within(confirmation).getByRole('button', { name: 'Delete' }));
    expect(DeleteMod).toHaveBeenCalledWith(custom.path, active.name);
    await screen.findByText('No mods in this game folder.');
  });

  it('saves a custom source and asset concurrency from the advanced drawer', async () => {
    vi.mocked(SetDownloadSettings).mockResolvedValue(
      ok({ source: 'custom', custom_shape: 'official', custom_url: 'https://mirror.example.com', concurrency: 12 }),
    );
    renderView();
    const summary = await screen.findByText('Download Settings');
    const drawer = summary.closest('details')!;
    expect(drawer.open).toBe(false);
    drawer.open = true;
    const source = await within(drawer).findByLabelText('Download source');
    expect(within(drawer).queryByLabelText('Custom API format')).toBeNull();
    fireEvent.change(source, { target: { value: 'custom' } });
    fireEvent.change(within(drawer).getByLabelText('Custom API format'), { target: { value: 'official' } });
    fireEvent.change(within(drawer).getByLabelText('Source base URL'), { target: { value: 'https://mirror.example.com' } });
    fireEvent.change(within(drawer).getByLabelText('Concurrent asset downloads'), { target: { value: '12' } });
    expect(within(drawer).getByText(/not multithreaded downloading/)).toBeTruthy();
    fireEvent.click(within(drawer).getByRole('button', { name: 'Save download settings' }));
    expect(SetDownloadSettings).toHaveBeenCalledWith('custom', 'official', 'https://mirror.example.com', 12);
    expect((await within(drawer).findByRole('status')).textContent).toBe('Download settings saved for future downloads.');
  });

  it('renders the Java installations with their paths', async () => {
    renderView();

    // JDK 17 shows up twice: the installation list and the paths section
    // (it is the default Java).
    expect(await screen.findAllByText('JDK 17')).toHaveLength(2);
    expect(screen.getByText('JDK 21')).toBeTruthy();
    expect(screen.getByText('17.0.9')).toBeTruthy();
    expect(screen.getByText('/opt/java21/bin/java')).toBeTruthy();
  });

  it('shows the empty notice when no Java is installed', async () => {
    mockGetJava.mockResolvedValue(ok([]));
    renderView();

    expect(await screen.findByText('No Java installations')).toBeTruthy();
  });

  it('surfaces a fetch error', async () => {
    mockGetJava.mockResolvedValue({ success: false, data: null, error: 'db offline' });
    renderView();

    expect(await screen.findByText('db offline')).toBeTruthy();
  });

  it('sets the default Java through the binding', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByLabelText('Set as default'));
    await waitFor(() => expect(mockSetDefault).toHaveBeenCalledWith(2));
  });

  it('deletes a Java installation after confirmation', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    const deleteButtons = screen.getAllByLabelText('Delete');
    fireEvent.click(deleteButtons[1]); // JDK 21

    const dialog = await screen.findByRole('alertdialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(mockDeleteJava).toHaveBeenCalledWith(2));
  });

  it('rejects an empty Java path in the add dialog', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Add Java' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add Java' }));

    expect(await within(dialog).findByText('Java path is required')).toBeTruthy();
    expect(mockAddJava).not.toHaveBeenCalled();
  });

  it('adds a Java installation from the dialog', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Add Java' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Java Path'), {
      target: { value: '/usr/lib/jvm/java-25/bin/java' },
    });
    fireEvent.change(within(dialog).getByLabelText('Friendly Name'), {
      target: { value: 'JDK 25' },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Add Java' }));

    await waitFor(() =>
      expect(mockAddJava).toHaveBeenCalledWith('/usr/lib/jvm/java-25/bin/java', 'JDK 25'),
    );
  });

  it('shows Java advice without changing the selected runtime', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Download Java' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByLabelText('Expected Minecraft version'), {
      target: { value: '1.18.2' },
    });
    expect(await within(dialog).findByText(/Java 17 is a suggested runtime/)).toBeTruthy();
    fireEvent.click(within(dialog).getByRole('button', { name: 'Next' }));

    const destination = await within(dialog).findByLabelText('Installation folder');
    await waitFor(() => expect((destination as HTMLInputElement).value).toBe('/launcher/jre/25'));
    const javaVersion = within(dialog).getByLabelText('Java version') as HTMLSelectElement;
    expect(javaVersion.value).toBe('25');
    fireEvent.change(javaVersion, { target: { value: '21' } });
    await waitFor(() => expect((destination as HTMLInputElement).value).toBe('/launcher/jre/21'));
    fireEvent.change(destination, { target: { value: '/custom/java/21' } });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Install Java' }));

    await waitFor(() => expect(mockInstallJavaRuntime).toHaveBeenCalledWith(21, '/custom/java/21'));
  });

  it('allows skipping the recommendation and installing directly', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Download Java' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Skip' }));
    await within(dialog).findByLabelText('Installation folder');
    await waitFor(() =>
      expect((within(dialog).getByLabelText('Installation folder') as HTMLInputElement).value)
        .toBe('/launcher/jre/25'),
    );

    fireEvent.click(within(dialog).getByRole('button', { name: 'Install Java' }));
    await waitFor(() => expect(mockInstallJavaRuntime).toHaveBeenCalledWith(25, '/launcher/jre/25'));
  });

  it('refreshes the default install path when the Java runtime changes', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Download Java' }));
    const dialog = await screen.findByRole('dialog');
    fireEvent.click(within(dialog).getByRole('button', { name: 'Next' }));
    const destination = await within(dialog).findByLabelText('Installation folder');
    await waitFor(() => expect((destination as HTMLInputElement).value).toBe('/launcher/jre/25'));
    fireEvent.change(within(dialog).getByLabelText('Java version'), { target: { value: '21' } });
    await waitFor(() => expect((destination as HTMLInputElement).value).toBe('/launcher/jre/21'));
    expect(mockGetJavaRuntimePath).toHaveBeenCalledWith(25);
    expect(mockGetJavaRuntimePath).toHaveBeenCalledWith(21);
  });

  it('saves memory settings through the binding', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    const maxInput = screen
      .getAllByLabelText('Maximum Memory (MB)')
      .find(el => el.tagName === 'INPUT') as HTMLInputElement;
    fireEvent.change(maxInput, { target: { value: '6144' } });

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(mockUpdateConfig).toHaveBeenCalledTimes(1));
    const payload = mockUpdateConfig.mock.calls[0][0] as { max_memory: number; min_memory: number };
    expect(payload.max_memory).toBe(6144);
    expect(payload.min_memory).toBe(1024);
    expect(await screen.findByText('Memory settings saved')).toBeTruthy();
  });

  it('blocks saving when minimum exceeds maximum memory', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    const minInput = screen
      .getAllByLabelText('Minimum Memory (MB)')
      .find(el => el.tagName === 'INPUT') as HTMLInputElement;
    fireEvent.change(minInput, { target: { value: '8192' } }); // max stays 4096

    fireEvent.click(screen.getByRole('button', { name: 'Save' }));
    expect(await screen.findByText('Minimum memory cannot exceed maximum memory')).toBeTruthy();
    expect(mockUpdateConfig).not.toHaveBeenCalled();
  });

  it('shows the auto-detected game directory with its source badge', async () => {
    renderView();

    expect(await screen.findByText('Auto-detected')).toBeTruthy();
    expect(screen.getByText('/home/player/.minecraft')).toBeTruthy();
  });

  it('warns when the game directory does not exist', async () => {
    const missing = { ...builtIn, path: '/missing/.minecraft', exists: false };
    mockGetGameDirStatus.mockResolvedValue(
      ok({ ...missing, dirs: [missing] }),
    );
    renderView();

    expect(
      await screen.findByText("This directory doesn't exist yet. Installing a version will create it."),
    ).toBeTruthy();
  });

  it('scans the disk and adds a found directory', async () => {
    mockScanGameDirs.mockResolvedValue(
      ok({
        dirs: [
          { path: '/games/mc/.minecraft', version_count: 2 },
          { path: '/home/player/.minecraft', version_count: 1 },
        ],
      }),
    );
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Scan disk' }));
    expect(await screen.findByText('/games/mc/.minecraft')).toBeTruthy();
    expect(screen.getByText('2 versions')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Added' }).hasAttribute('disabled')).toBe(true);

    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    await waitFor(() => expect(mockAddGameDir).toHaveBeenCalledWith(custom.path));
  });

  it('shows the empty notice when the scan finds nothing', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Scan disk' }));
    expect(
      await screen.findByText('No game directories found. You can specify one manually.'),
    ).toBeTruthy();
  });

  it('adds a typed path without replacing the default folder', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Add folder' }));
    fireEvent.change(screen.getByLabelText('Game Directory'), {
      target: { value: '  /games/mc/.minecraft  ' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Add folder' }));

    expect(await screen.findByText(custom.path)).toBeTruthy();
    expect(mockAddGameDir).toHaveBeenCalledWith(custom.path);
    expect(within(screen.getByText(builtIn.path).closest('li')!).getByText('Default for new installs')).toBeTruthy();
  });

  it('sets a saved directory as the install default', async () => {
    mockGetGameDirStatus.mockResolvedValue(ok({ ...builtIn, dirs: [builtIn, custom] }));
    renderView();

    expect(await screen.findByText('Custom')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Set as install default' }));
    await waitFor(() => expect(mockSetGameDir).toHaveBeenCalledWith(custom.path));
    expect(within(screen.getByText(custom.path).closest('li')!).getByText('Default for new installs')).toBeTruthy();
  });

  it('removes a non-default path from the folder list', async () => {
    mockGetGameDirStatus.mockResolvedValue(ok({ ...builtIn, dirs: [builtIn, custom] }));
    renderView();
    fireEvent.click(await screen.findByRole('button', { name: `Remove ${custom.path} from the folder list` }));
    await waitFor(() => expect(screen.queryByText(custom.path)).toBeNull());
    expect(mockRemoveGameDir).toHaveBeenCalledWith(custom.path);
  });
});
