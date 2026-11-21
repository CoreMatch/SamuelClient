import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, within } from '@testing-library/react';
import { GetInstalledVersions } from '../../wailsjs/go/main/App';
import VersionManager from './VersionManager';
import type { InstalledVersion } from '../types';

// The wizard is covered by its own suite; stub it so the manager tests stay
// hermetic (no version-list bindings).
vi.mock('./InstallWizard', () => ({
  default: ({ onInstalled }: { onInstalled: () => void }) => (
    <button onClick={onInstalled}>Finish install</button>
  ),
}));

vi.mock('../../wailsjs/go/main/App', () => ({
  GetInstalledVersions: vi.fn(),
}));

const mockGetInstalledVersions = vi.mocked(GetInstalledVersions);
const STORAGE_KEY = 'installed_versions';

const versions: InstalledVersion[] = [
  { id: '1.20.4', type: 'release', installed: true },
  { id: '1.20.4-forge49.0.3', type: 'release', installed: true, loaderType: 'forge', loaderVersion: '49.0.3' },
];

function openInstalledTab(): void {
  // Radix Tabs activates on mousedown, not click.
  fireEvent.mouseDown(screen.getByRole('tab', { name: 'Installed' }));
}

describe('VersionManager', () => {
  beforeEach(() => {
    localStorage.clear();
    mockGetInstalledVersions.mockReset();
    mockGetInstalledVersions.mockResolvedValue({ success: true, data: { versions: [] }, error: '' });
  });

  it('finds installed versions on disk without a local list', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: [
        { id: '1.16.5', type: 'release', has_json: true, has_jar: true },
        { id: 'incomplete', type: 'release', has_json: true, has_jar: false },
      ] },
      error: '',
    });
    const onSelectVersion = vi.fn();
    render(<VersionManager onSelectVersion={onSelectVersion} />);

    openInstalledTab();
    fireEvent.click(await screen.findByText('1.16.5'));
    expect(onSelectVersion).toHaveBeenCalledWith({ id: '1.16.5', type: 'release', installed: true });
    expect(screen.queryByText('incomplete')).toBeNull();
    expect(screen.queryByLabelText('Remove 1.16.5 from the list')).toBeNull();
  });

  it('scans for versions after the wizard finishes', async () => {
    render(<VersionManager onSelectVersion={vi.fn()} />);

    fireEvent.click(screen.getByText('Finish install'));
    await waitFor(() => expect(mockGetInstalledVersions).toHaveBeenCalledOnce());
  });

  it('lists installed versions and reports the selection', async () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(versions));
    const onSelectVersion = vi.fn();
    render(<VersionManager onSelectVersion={onSelectVersion} />);

    openInstalledTab();
    const moddedId = await screen.findByText('1.20.4-forge49.0.3');
    expect(screen.getByText('1.20.4')).toBeTruthy();
    // The loader badge carries type and version.
    expect(screen.getByText('forge 49.0.3')).toBeTruthy();

    // Click the id text; the event bubbles to the row's select handler.
    fireEvent.click(moddedId);
    expect(onSelectVersion).toHaveBeenCalledWith(versions[1]);
  });

  it('removes a version from the list and from localStorage', async () => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(versions));
    render(<VersionManager onSelectVersion={vi.fn()} />);

    openInstalledTab();
    fireEvent.click(await screen.findByLabelText('Remove 1.20.4 from the list'));

    expect(screen.queryByText('1.20.4')).toBeNull();
    expect(screen.getByText('1.20.4-forge49.0.3')).toBeTruthy();
    const stored = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '[]');
    expect(stored.map((v: InstalledVersion) => v.id)).toEqual(['1.20.4-forge49.0.3']);
  });

  it('shows the empty state when nothing is installed', async () => {
    render(<VersionManager onSelectVersion={vi.fn()} />);

    openInstalledTab();
    expect(await screen.findByText('No versions installed')).toBeTruthy();
  });

  it('persists a nickname for only the edited game directory and clears it when blank', async () => {
    mockGetInstalledVersions.mockResolvedValue({
      success: true,
      data: { versions: ['/games/main', '/games/other'].map(game_dir => ({
        id: '1.20.4', type: 'release', has_json: true, has_jar: true, game_dir,
      })) },
      error: '',
    });
    const onSelectVersion = vi.fn();
    const first = render(<VersionManager onSelectVersion={onSelectVersion} />);

    openInstalledTab();
    const editLabel = 'Edit nickname for 1.20.4 (/games/main)';
    const editButton = await screen.findByRole('button', { name: editLabel });
    fireEvent.click(editButton);
    const input = screen.getByRole('textbox', { name: 'Nickname' });
    expect(document.activeElement).toBe(input);
    fireEvent.change(input, { target: { value: '  Main world  ' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save nickname' }));
    expect(document.activeElement).toBe(editButton);
    expect(onSelectVersion).not.toHaveBeenCalled();
    first.unmount();

    render(<VersionManager onSelectVersion={onSelectVersion} />);
    openInstalledTab();
    const renamed = await screen.findByRole('button', { name: 'Select Main world, 1.20.4 (/games/main)' });
    expect(within(renamed).getByText('1.20.4')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Select 1.20.4 (/games/other)' })).toBeTruthy();
    fireEvent.click(renamed);
    expect(onSelectVersion).toHaveBeenCalledWith({
      id: '1.20.4', type: 'release', installed: true, gameDir: '/games/main',
    });

    fireEvent.click(screen.getByRole('button', { name: editLabel }));
    const savedInput = screen.getByRole<HTMLInputElement>('textbox', { name: 'Nickname' });
    expect(savedInput.value).toBe('Main world');
    fireEvent.change(savedInput, { target: { value: 'Discard me' } });
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(screen.getByText('Main world')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: editLabel }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Nickname' }), { target: { value: '   ' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save nickname' }));
    expect(screen.queryByText('Main world')).toBeNull();
    expect(screen.getByRole('button', { name: 'Select 1.20.4 (/games/main)' })).toBeTruthy();
  });
});
