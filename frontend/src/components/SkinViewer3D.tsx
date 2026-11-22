import { useEffect, useRef, useState } from 'react';
import { SkinViewer } from 'skinview3d';
import { t } from '../i18n';

interface SkinViewer3DProps {
  skinUrl?: string | null;
  capeUrl?: string | null;
  skinFallbackUrl?: string;
  capeFallbackUrl?: string;
  width?: number;
  height?: number;
}

const SIDE_BACK_ROTATION = Math.PI * 0.72;

async function loadImageFromUrl(url: string): Promise<{
  image: HTMLImageElement;
  revoke: () => void;
}> {
  const response = await fetch(url, { cache: 'no-store' });
  if (!response.ok) throw new Error(`Texture request failed (${response.status})`);

  const objectUrl = URL.createObjectURL(await response.blob());
  const image = new Image();
  return new Promise((resolve, reject) => {
    image.onload = () => resolve({
      image,
      revoke: () => URL.revokeObjectURL(objectUrl),
    });
    image.onerror = () => {
      URL.revokeObjectURL(objectUrl);
      reject(new Error('Texture image could not be decoded'));
    };
    image.src = objectUrl;
  });
}

function normalizeCapeTexture(image: HTMLImageElement): HTMLImageElement | HTMLCanvasElement {
  const width = image.naturalWidth || image.width;
  const height = image.naturalHeight || image.height;
  if (width === height * 2) return image;

  let scale: number | null = null;
  if (width * 17 === height * 22) scale = width / 22;
  else if (width * 11 === height * 23) scale = width / 46;
  if (!scale) throw new Error(`Unsupported cape texture size: ${width}x${height}`);

  const canvas = document.createElement('canvas');
  canvas.width = 64 * scale;
  canvas.height = 32 * scale;
  const context = canvas.getContext('2d');
  if (!context) throw new Error('Could not create cape texture canvas');
  context.drawImage(image, 0, 0, width, height);
  return canvas;
}

async function loadTexture(url: string | null | undefined, fallbackUrl?: string) {
  if (!url && fallbackUrl) return loadImageFromUrl(fallbackUrl);
  if (!url) throw new Error('Texture URL is missing');
  try {
    return await loadImageFromUrl(url);
  } catch (error) {
    if (!fallbackUrl || fallbackUrl === url) throw error;
    return loadImageFromUrl(fallbackUrl);
  }
}

export default function SkinViewer3D({
  skinUrl,
  capeUrl,
  skinFallbackUrl,
  capeFallbackUrl,
  width = 260,
  height = 360,
}: SkinViewer3DProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const viewerRef = useRef<SkinViewer | null>(null);
  const [viewerError, setViewerError] = useState(false);
  const [skinError, setSkinError] = useState(false);
  const [capeError, setCapeError] = useState(false);

  useEffect(() => {
    if (!canvasRef.current) return;
    try {
      const viewer = new SkinViewer({
        canvas: canvasRef.current,
        width,
        height,
        zoom: 0.55,
        enableControls: true,
      });
      viewer.autoRotate = true;
      viewer.autoRotateSpeed = 0.5;
      viewer.playerWrapper.rotation.y = SIDE_BACK_ROTATION;
      viewerRef.current = viewer;
      setViewerError(false);

      return () => {
        viewer.dispose();
        viewerRef.current = null;
      };
    } catch (error) {
      console.warn('Could not initialize the 3D profile preview:', error);
      setViewerError(true);
    }
  }, [width, height]);

  useEffect(() => {
    setSkinError(false);
    const viewer = viewerRef.current;
    if (!viewer) return;
    let disposed = false;
    let revoke: (() => void) | null = null;

    const applySkin = async () => {
      if (!skinUrl && !skinFallbackUrl) {
        viewer.loadSkin(null);
        return;
      }
      try {
        const loaded = await loadTexture(skinUrl, skinFallbackUrl);
        if (disposed) {
          loaded.revoke();
          return;
        }
        revoke = loaded.revoke;
        viewer.loadSkin(loaded.image);
      } catch (error) {
        if (!disposed) {
          viewer.loadSkin(null);
          console.warn('Could not load the profile skin:', error);
          setSkinError(true);
        }
      }
    };

    void applySkin();
    return () => {
      disposed = true;
      revoke?.();
    };
  }, [skinUrl, skinFallbackUrl, width, height]);

  useEffect(() => {
    setCapeError(false);
    const viewer = viewerRef.current;
    if (!viewer) return;
    let disposed = false;
    let revoke: (() => void) | null = null;

    const applyCape = async () => {
      if (!capeUrl && !capeFallbackUrl) {
        viewer.loadCape(null);
        viewer.playerObject.backEquipment = null;
        return;
      }
      try {
        const loaded = await loadTexture(capeUrl, capeFallbackUrl);
        if (disposed) {
          loaded.revoke();
          return;
        }
        revoke = loaded.revoke;
        viewer.loadCape(normalizeCapeTexture(loaded.image), { backEquipment: 'cape' });
        viewer.playerObject.backEquipment = 'cape';
      } catch (error) {
        if (!disposed) {
          viewer.loadCape(null);
          console.warn('Could not load the profile cape:', error);
          setCapeError(true);
        }
      }
    };

    void applyCape();
    return () => {
      disposed = true;
      revoke?.();
    };
  }, [capeUrl, capeFallbackUrl, width, height]);

  return (
    <div className="relative flex min-h-[300px] items-center justify-center overflow-hidden rounded-md bg-panel">
      {(viewerError || skinError || capeError) && (
        <div role="status" className="absolute inset-x-3 bottom-3 z-10 rounded bg-panel/90 p-2 text-center text-xs text-ink-muted">
          {viewerError ? t('profile.previewRendererFailed') : (
            <>
              {skinError && <p>{t('profile.previewSkinFailed')}</p>}
              {capeError && <p>{t('profile.previewCapeFailed')}</p>}
            </>
          )}
        </div>
      )}
      <canvas
        ref={canvasRef}
        width={width}
        height={height}
        aria-label="Interactive 3D player preview"
        className="max-w-full"
        style={{ imageRendering: 'pixelated' }}
      />
    </div>
  );
}
