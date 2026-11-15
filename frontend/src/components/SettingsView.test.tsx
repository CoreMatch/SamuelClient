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
  GetGameDirStatus,
  SetGameDir,
  ScanGameDirs,
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
  GetGameDirStatus: vi.fn(),
  SetGameDir: vi.fn(),
  ScanGameDirs: vi.fn(),
}));

const mockGetConfig = vi.mocked(GetLauncherConfig);
const mockGetJava = vi.mocked(GetJavaInstallations);
const mockUpdateConfig = vi.mocked(UpdateLauncherConfig);
const mockAddJava = vi.mocked(AddJavaInstallation);
const mockUpdateJava = vi.mocked(UpdateJavaInstallation);
const mockDeleteJava = vi.mocked(DeleteJavaInstallation);
const mockSetDefault = vi.mocked(SetDefaultJavaInstallation);
const mockGetGameDirStatus = vi.mocked(GetGameDirStatus);
const mockSetGameDir = vi.mocked(SetGameDir);
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
    mockDeleteJava.mockResolvedValue(ok(javaRows));
    mockAddJava.mockResolvedValue(ok(javaRows));
    mockUpdateJava.mockResolvedValue(ok(javaRows));
    mockUpdateConfig.mockResolvedValue(ok({ ...launcherConfig, max_memory: 6144 }));
    mockGetGameDirStatus.mockResolvedValue(
      ok({ path: '/home/player/.minecraft', source: 'auto', exists: true }),
    );
    mockSetGameDir.mockResolvedValue(
      ok({ path: '/games/mc/.minecraft', source: 'custom', exists: true }),
    );
    mockScanGameDirs.mockResolvedValue(ok({ dirs: [] }));
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
    mockGetGameDirStatus.mockResolvedValue(
      ok({ path: '/missing/.minecraft', source: 'auto', exists: false }),
    );
    renderView();

    expect(
      await screen.findByText("This directory doesn't exist yet. Installing a version will create it."),
    ).toBeTruthy();
  });

  it('scans the disk and applies a found directory', async () => {
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
    // The directory already in use is marked and cannot be re-applied.
    expect(screen.getByRole('button', { name: 'In use' })).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Use' }));
    await waitFor(() => expect(mockSetGameDir).toHaveBeenCalledWith('/games/mc/.minecraft'));
  });

  it('shows the empty notice when the scan finds nothing', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Scan disk' }));
    expect(
      await screen.findByText('No game directories found. You can specify one manually.'),
    ).toBeTruthy();
  });

  it('specifies a custom directory through the editor', async () => {
    renderView();
    await screen.findAllByText('JDK 17');

    fireEvent.click(screen.getByRole('button', { name: 'Specify directory' }));
    fireEvent.change(screen.getByLabelText('Game Directory'), {
      target: { value: '  /games/mc/.minecraft  ' },
    });
    // The editor's Save button renders before the memory section's Save.
    const saveButtons = screen.getAllByRole('button', { name: 'Save' });
    fireEvent.click(saveButtons[0]);

    await waitFor(() => expect(mockSetGameDir).toHaveBeenCalledWith('/games/mc/.minecraft'));
  });

  it('resets a custom directory to auto-detection', async () => {
    mockGetGameDirStatus.mockResolvedValue(
      ok({ path: '/games/mc/.minecraft', source: 'custom', exists: true }),
    );
    renderView();

    expect(await screen.findByText('Custom')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Use auto-detection' }));
    await waitFor(() => expect(mockSetGameDir).toHaveBeenCalledWith(''));
  });
});
