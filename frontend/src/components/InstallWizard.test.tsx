import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import {
  GetVersions,
  GetFabricVersions,
  GetForgeVersions,
  DownloadVersion,
  InstallFabricLoader,
  InstallForgeLoader,
  CancelInstall,
} from '../../wailsjs/go/main/App';
import InstallWizard from './InstallWizard';

// All Wails bindings are test doubles; the wizard must never reach a real
// backend. The runtime event bus is stubbed out too.
vi.mock('../../wailsjs/go/main/App', () => ({
  GetVersions: vi.fn(),
  GetFabricVersions: vi.fn(),
  GetForgeVersions: vi.fn(),
  DownloadVersion: vi.fn(),
  InstallFabricLoader: vi.fn(),
  InstallForgeLoader: vi.fn(),
  CancelInstall: vi.fn(),
}));
vi.mock('../../wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(),
  EventsOff: vi.fn(),
}));

const mockGetVersions = vi.mocked(GetVersions);
const mockGetFabricVersions = vi.mocked(GetFabricVersions);
const mockGetForgeVersions = vi.mocked(GetForgeVersions);
const mockDownloadVersion = vi.mocked(DownloadVersion);
const mockInstallFabricLoader = vi.mocked(InstallFabricLoader);
const mockInstallForgeLoader = vi.mocked(InstallForgeLoader);
const mockCancelInstall = vi.mocked(CancelInstall);

function mockManifest() {
  mockGetVersions.mockResolvedValue({
    success: true,
    data: { versions: [{ id: '1.99', type: 'release' }, { id: '1.98', type: 'release' }] },
    error: '',
  });
}

async function selectVersionAndNext() {
  const row = await screen.findByText('1.99');
  fireEvent.click(row);
  fireEvent.click(screen.getByText('Next'));
}

describe('InstallWizard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockManifest();
    mockGetFabricVersions.mockResolvedValue({ success: true, data: [{ loader: { version: '0.15.0' } }], error: '' });
    mockGetForgeVersions.mockResolvedValue({ success: true, data: [{ version: '47.2.0' }], error: '' });
    mockDownloadVersion.mockResolvedValue({ success: true, data: null, error: '' });
    mockInstallFabricLoader.mockResolvedValue({ success: true, data: { version_id: 'fabric-loader-0.15.0-1.99' }, error: '' });
    mockInstallForgeLoader.mockResolvedValue({ success: true, data: { version_id: '1.99-forge-47.2.0' }, error: '' });
  });

  it('walks the three steps and installs a vanilla version', async () => {
    const onInstalled = vi.fn();
    render(<InstallWizard onInstalled={onInstalled} />);

    // Step 1: version list.
    await selectVersionAndNext();

    // Step 2: loader cards, vanilla preselected.
    expect(screen.getByText('Vanilla')).toBeTruthy();
    fireEvent.click(screen.getAllByText('Next')[0]);

    // Step 3: confirm and install.
    expect(screen.getByText('Minecraft 1.99 (vanilla).')).toBeTruthy();
    fireEvent.click(screen.getByText('Install'));

    await waitFor(() => expect(mockDownloadVersion).toHaveBeenCalledWith('1.99', ''));
    expect(mockInstallFabricLoader).not.toHaveBeenCalled();
    expect(mockInstallForgeLoader).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.getByText(/Installed 1\.99/)).toBeTruthy());
    expect(onInstalled).toHaveBeenCalled();
  });

  it('installs Fabric through the loader step', async () => {
    render(<InstallWizard onInstalled={vi.fn()} />);
    await selectVersionAndNext();

    // Loader versions were fetched for the selected Minecraft version.
    await waitFor(() => expect(mockGetFabricVersions).toHaveBeenCalledWith('1.99'));

    fireEvent.click(screen.getByText('Fabric'));
    fireEvent.click(screen.getAllByText('Next')[0]);

    expect(screen.getByText(/Minecraft 1\.99 with Fabric 0\.15\.0/)).toBeTruthy();
    fireEvent.click(screen.getByText('Install'));

    await waitFor(() => expect(mockInstallFabricLoader).toHaveBeenCalledWith('1.99', '0.15.0', ''));
    await waitFor(() => expect(screen.getByText(/Installed fabric-loader-0\.15\.0-1\.99/)).toBeTruthy());
  });

  it('shows a readable failure with retry when the download fails', async () => {
    mockDownloadVersion.mockResolvedValue({ success: false, data: null, error: 'mirror down' });
    render(<InstallWizard onInstalled={vi.fn()} />);
    await selectVersionAndNext();
    fireEvent.click(screen.getAllByText('Next')[0]);
    fireEvent.click(screen.getByText('Install'));

    await waitFor(() => expect(screen.getByText(/Installation failed: mirror down/)).toBeTruthy());
    fireEvent.click(screen.getByText('Retry'));
    await waitFor(() => expect(mockDownloadVersion).toHaveBeenCalledTimes(2));
  });

  it('cancels an in-flight install and finalizes cleanly', async () => {
    // Hold the download open so the wizard stays in the installing phase.
    let resolveDownload!: (v: unknown) => void;
    const pending = new Promise(resolve => {
      resolveDownload = resolve;
    });
    mockDownloadVersion.mockReturnValue(pending as ReturnType<typeof DownloadVersion>);
    mockCancelInstall.mockResolvedValue({ success: true, data: { cancelled: true }, error: '' });

    render(<InstallWizard onInstalled={vi.fn()} />);
    await selectVersionAndNext();
    fireEvent.click(screen.getAllByText('Next')[0]);
    fireEvent.click(screen.getByText('Install'));

    // Installing: a cancel control is offered.
    fireEvent.click(await screen.findByText('Cancel'));
    await waitFor(() => expect(mockCancelInstall).toHaveBeenCalledWith('1.99'));

    // The backend settles the cancelled run with a failure; the wizard must
    // finalize to the cancelled state rather than a scary error.
    resolveDownload({ success: false, data: null, error: 'install cancelled' });
    await waitFor(() => expect(screen.getByText(/Installation cancelled/)).toBeTruthy());
    expect(screen.queryByText(/Installation failed/)).toBeNull();
  });

  it('shows the unified error state when the manifest fails', async () => {
    mockGetVersions.mockResolvedValue({ success: false, data: null, error: 'down' });
    render(<InstallWizard onInstalled={vi.fn()} />);
    await waitFor(() =>
      expect(screen.getByText("Couldn't load the version list.")).toBeTruthy(),
    );
    // The async act lets the retried fetch settle inside act (no console
    // noise after the test body ends).
    await act(async () => {
      fireEvent.click(screen.getByText('Retry'));
    });
    expect(mockGetVersions).toHaveBeenCalledTimes(2);
  });
});
