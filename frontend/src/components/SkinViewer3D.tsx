import { useEffect, useRef, useState } from 'react';
import { SkinViewer } from 'skinview3d';

interface SkinViewer3DProps {
  skinUrl?: string | null;
  capeUrl?: string | null;
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

export default function SkinViewer3D({
  skinUrl,
  capeUrl,
  width = 260,
  height = 360,
}: SkinViewer3DProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const viewerRef = useRef<SkinViewer | null>(null);
  const [viewerError, setViewerError] = useState(false);

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
    } catch {
      setViewerError(true);
    }
  }, [width, height]);

  useEffect(() => {
    const viewer = viewerRef.current;
    if (!viewer) return;
    let disposed = false;
    let revoke: (() => void) | null = null;

    const applySkin = async () => {
      if (!skinUrl) {
        viewer.loadSkin(null);
        return;
      }
      try {
        const loaded = await loadImageFromUrl(skinUrl);
        if (disposed) {
          loaded.revoke();
          return;
        }
        revoke = loaded.revoke;
        viewer.loadSkin(loaded.image);
      } catch {
        if (!disposed) setViewerError(true);
      }
    };

    void applySkin();
    return () => {
      disposed = true;
      revoke?.();
    };
  }, [skinUrl]);

  useEffect(() => {
    const viewer = viewerRef.current;
    if (!viewer) return;
    let disposed = false;
    let revoke: (() => void) | null = null;

    const applyCape = async () => {
      if (!capeUrl) {
        viewer.loadCape(null);
        viewer.playerObject.backEquipment = null;
        return;
      }
      try {
        const loaded = await loadImageFromUrl(capeUrl);
        if (disposed) {
          loaded.revoke();
          return;
        }
        revoke = loaded.revoke;
        viewer.loadCape(normalizeCapeTexture(loaded.image), { backEquipment: 'cape' });
        viewer.playerObject.backEquipment = 'cape';
      } catch {
        if (!disposed) setViewerError(true);
      }
    };

    void applyCape();
    return () => {
      disposed = true;
      revoke?.();
    };
  }, [capeUrl]);

  return (
    <div className="relative flex min-h-[300px] items-center justify-center overflow-hidden rounded-md bg-panel">
      {viewerError && (
        <span className="absolute inset-x-3 bottom-3 z-10 rounded bg-panel/90 p-2 text-center text-xs text-ink-muted">
          3D preview unavailable
        </span>
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
