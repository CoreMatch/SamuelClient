import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import VersionManager from './VersionManager';
import type { InstalledVersion } from '../types';

// The wizard is covered by its own suite; stub it so the manager tests stay
// hermetic (no version-list bindings).
vi.mock('./InstallWizard', () => ({
  default: () => <div data-testid="install-wizard" />,
}));

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
});
