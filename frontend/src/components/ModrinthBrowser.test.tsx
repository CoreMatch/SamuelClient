import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { GetCurseForgeTargets, GetModrinthTargets, GetModrinthVersions, InstallModrinthMod, PreviewModrinthInstall, SearchModrinth } from '../../wailsjs/go/main/App';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
import ModrinthBrowser from './ModrinthBrowser';

vi.mock('../../wailsjs/go/main/App', () => ({
  GetCurseForgeTargets: vi.fn(),
  GetModrinthTargets: vi.fn(),
  GetModrinthVersions: vi.fn(),
  InstallModrinthMod: vi.fn(),
  PreviewModrinthInstall: vi.fn(),
  SearchModrinth: vi.fn(),
}));
vi.mock('../../wailsjs/runtime/runtime', () => ({ BrowserOpenURL: vi.fn() }));

const ok = (data: unknown) => ({ success: true, data, error: '' });

describe('ModrinthBrowser', () => {
  it('keeps the official CurseForge website available without a key', async () => {
    vi.mocked(GetCurseForgeTargets).mockResolvedValue({
      success: false, data: null, error: 'Set SAMUEL_CURSEFORGE_API_KEY before launching.',
    });
    render(<ModrinthBrowser provider="curseforge" gameDir="/games/mc" busy={false}
      setBusy={vi.fn()} onInstalled={vi.fn(async () => {})} />);

    await screen.findByText(/Set SAMUEL_CURSEFORGE_API_KEY/);
    expect(screen.queryByLabelText('Mod name or keyword')).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: 'Open CurseForge website' }));
    expect(BrowserOpenURL).toHaveBeenCalledWith('https://www.curseforge.com/minecraft/search?class=mc-mods');
  });

  it('shows required files and waits for confirmation before installing', async () => {
    const target = 'fabric-loader-0.16-1.20.1';
    vi.mocked(GetModrinthTargets).mockResolvedValue(ok([
      { profile_id: target, game_version: '1.20.1', loader: 'fabric' },
    ]));
    vi.mocked(SearchModrinth).mockResolvedValue(ok({
      hits: [{ project_id: 'projectA', title: 'Example', author: 'Author', description: 'A mod' }],
      limit: 20, total_hits: 1,
    }));
    vi.mocked(GetModrinthVersions).mockResolvedValue(ok([
      { id: 'versionA', version_number: '1.0', version_type: 'release' },
    ]));
    vi.mocked(PreviewModrinthInstall).mockResolvedValue(ok({
      fingerprint: 'approved',
      files: [
        { name: 'example.jar', version: '1.0', channel: 'release', size: 1024 },
        { name: 'dependency.jar', version: '2.0', channel: 'release', size: 2048 },
      ],
    }));
    vi.mocked(InstallModrinthMod).mockResolvedValue(ok(null));
    const onInstalled = vi.fn(async () => {});
    render(<ModrinthBrowser gameDir="/games/mc" busy={false} setBusy={vi.fn()} onInstalled={onInstalled} />);

    fireEvent.change(await screen.findByLabelText('Mod name or keyword'), { target: { value: 'Example' } });
    fireEvent.click(screen.getByRole('button', { name: 'Search' }));
    expect(SearchModrinth).toHaveBeenCalledWith('/games/mc', target, 'Example', 0);
    fireEvent.click(await screen.findByRole('button', { name: 'View versions for Example' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Review installation' }));
    await screen.findByText(/Required dependency: dependency\.jar/);
    expect(InstallModrinthMod).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Confirm and install' }));
    await screen.findByText('Modrinth installation complete.');
    expect(InstallModrinthMod).toHaveBeenCalledWith('/games/mc', target, 'projectA', 'versionA', 'approved');
    expect(onInstalled).toHaveBeenCalledOnce();
  });
});
