import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { InstallModpack, PickModpack, PreviewModpack } from '../../wailsjs/go/main/App';
import ModpackBrowser from './ModpackBrowser';

vi.mock('../../wailsjs/go/main/App', () => ({
  CancelInstall: vi.fn(),
  GetModrinthModpackVersions: vi.fn(),
  InstallModpack: vi.fn(),
  PickModpack: vi.fn(),
  PreviewModpack: vi.fn(),
  SearchModrinthModpacks: vi.fn(),
}));
vi.mock('../../wailsjs/runtime/runtime', () => ({ EventsOn: vi.fn(() => () => {}) }));

describe('ModpackBrowser', () => {
  it('previews a local archive and installs only the selected optional files after confirmation', async () => {
    vi.mocked(PickModpack).mockResolvedValue({ success: true, data: '/local/demo.mrpack' });
    vi.mocked(PreviewModpack).mockResolvedValue({ success: true, data: {
      name: 'Demo', version: 'v1', game_version: '1.20.1', loader: 'fabric', loader_version: '0.16.0',
      required_bytes: 256, fingerprint: 'approved', options: [
        { path: 'mods/selected.jar', size: 1024 },
        { path: 'mods/not-selected.jar', size: 2048 },
      ],
    } });
    vi.mocked(InstallModpack).mockResolvedValue({ success: true, data: {
      name: 'Demo', version_id: 'fabric-loader-0.16.0-1.20.1', game_dir: '/instances/demo',
    } });
    const onInstalled = vi.fn();
    const onSelectVersion = vi.fn();
    render(<ModpackBrowser onInstalled={onInstalled} onSelectVersion={onSelectVersion} />);
    fireEvent.click(screen.getByRole('button', { name: 'Open local .mrpack' }));
    const option = await screen.findByRole('checkbox', { name: 'mods/selected.jar' });
    expect(PreviewModpack).toHaveBeenCalledWith('', '', '/local/demo.mrpack');
    expect(InstallModpack).not.toHaveBeenCalled();
    fireEvent.click(option);
    fireEvent.click(screen.getByRole('button', { name: 'Install as a new instance' }));
    expect(InstallModpack).toHaveBeenCalledWith(
      expect.stringMatching(/^modpack-[a-f0-9]{32}$/), '', '', '/local/demo.mrpack', 'approved', ['mods/selected.jar'],
    );
    fireEvent.click(await screen.findByRole('button', { name: 'Use as selected version' }));
    expect(onInstalled).toHaveBeenCalledOnce();
    expect(onSelectVersion).toHaveBeenCalledWith(expect.objectContaining({
      modpackName: 'Demo', gameDir: '/instances/demo', loaderType: 'fabric',
    }));
  });
});
