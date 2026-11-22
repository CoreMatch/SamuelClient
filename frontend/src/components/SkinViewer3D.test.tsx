import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import SkinViewer3D from './SkinViewer3D';

const { construct, loadSkin, loadCape } = vi.hoisted(() => ({
  construct: vi.fn(),
  loadSkin: vi.fn(),
  loadCape: vi.fn(),
}));

vi.mock('skinview3d', () => ({
  SkinViewer: class {
    playerWrapper = { rotation: { y: 0 } };
    playerObject = { backEquipment: null };
    loadSkin = loadSkin;
    loadCape = loadCape;
    dispose = vi.fn();

    constructor() {
      construct();
    }
  },
}));

const fetchTexture = vi.fn();

beforeEach(() => {
  vi.resetAllMocks();
  vi.spyOn(console, 'warn').mockImplementation(() => {});
  vi.stubGlobal('fetch', fetchTexture);
  vi.stubGlobal('URL', class extends URL {
    static createObjectURL = vi.fn(() => 'blob:test');
    static revokeObjectURL = vi.fn();
  });
  vi.stubGlobal('Image', class {
    onload?: () => void;
    set src(_value: string) {
      queueMicrotask(() => this.onload?.());
    }
  });
  fetchTexture.mockImplementation(async (url: string) =>
    url.endsWith('/missing') ? { ok: false, status: 404 } : { ok: true, blob: async () => new Blob() },
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('SkinViewer3D', () => {
  it('reports renderer initialization failures separately', () => {
    construct.mockImplementationOnce(() => { throw new Error('WebGL not available'); });
    render(<SkinViewer3D skinUrl="/skin" />);

    expect(screen.getByRole('status').textContent).toBe('Could not start the 3D renderer.');
    expect(fetchTexture).not.toHaveBeenCalled();
  });

  it('keeps the skin visible when the cape fails to load', async () => {
    render(<SkinViewer3D skinUrl="/skin" capeUrl="/cape/missing" />);

    expect((await screen.findByRole('status')).textContent).toBe('Could not load the cape preview.');
    await waitFor(() => expect(loadSkin).toHaveBeenCalledWith(expect.any(Image)));
    expect(loadSkin).not.toHaveBeenCalledWith(null);
  });

  it('renders the skin from its hash URL when the saved URL fails', async () => {
    render(<SkinViewer3D skinUrl="/legacy/missing" skinFallbackUrl="/yggdrasil-api/textures/skin" />);

    await waitFor(() => expect(loadSkin).toHaveBeenCalledWith(expect.any(Image)));
    expect(fetchTexture.mock.calls.map(([url]) => url)).toEqual([
      '/legacy/missing',
      '/yggdrasil-api/textures/skin',
    ]);
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('clears a skin error when the replacement texture loads', async () => {
    const { rerender } = render(<SkinViewer3D skinUrl="/skin/missing" />);

    expect((await screen.findByRole('status')).textContent).toBe('Could not load the skin preview.');
    rerender(<SkinViewer3D skinUrl="/skin/replacement" />);

    await waitFor(() => expect(loadSkin).toHaveBeenCalledWith(expect.any(Image)));
    expect(screen.queryByRole('status')).toBeNull();
  });
});
