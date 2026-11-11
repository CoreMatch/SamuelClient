import { useEffect, useState } from 'react';
import { Asset } from '@ha/ui';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';

// useInstallProgress is the single shared subscription for the
// "download:progress" Wails event (ADR-0004). Both the InstallWizard and the
// HomeView loader quick-install render their progress bars from it, so the
// two paths share one contract interpretation instead of drifting apart.
//
// Semantics (identical for every consumer):
//   - total > 0  -> real byte percentage, floored and clamped to 100
//   - total <= 0 -> indeterminate stage (pct stays null; the bar pulses)
//   - terminal "cancelled" stage -> the entry is deleted so the bar
//     finalizes instead of stalling mid-progress (no residual rows)

export interface InstallProgress {
  stage: string;
  pct: number | null;
}

interface DownloadProgressPayload {
  version_id?: string;
  stage?: string;
  downloaded?: number;
  total?: number;
}

export function useInstallProgress() {
  const [progress, setProgress] = useState<Record<string, InstallProgress>>({});

  useEffect(() => {
    EventsOn('download:progress', (p: DownloadProgressPayload) => {
      if (!p || !p.version_id) return;
      // A terminal "cancelled" stage clears the entry so the bar finalizes
      // instead of stalling mid-progress.
      if (p.stage === 'cancelled') {
        setProgress(prev => {
          const next = { ...prev };
          delete next[p.version_id as string];
          return next;
        });
        return;
      }
      let pct: number | null = null;
      if (p.total && p.total > 0 && typeof p.downloaded === 'number') {
        pct = Math.min(100, Math.floor((p.downloaded / p.total) * 100));
      }
      setProgress(prev => ({ ...prev, [p.version_id as string]: { stage: p.stage || '', pct } }));
    });
    return () => {
      EventsOff('download:progress');
    };
  }, []);

  const clearProgress = () => setProgress({});

  return { progress, clearProgress };
}

// Maps a "download:progress" stage to its i18n key; unknown stages fall
// back to the generic game-file label instead of rendering a raw key.
export function installStepKey(
  stage: string,
): 'install.step.jar' | 'install.step.library' | 'install.step.native' | 'install.step.asset' | 'install.step.loader' {
  switch (stage) {
    case 'library':
      return 'install.step.library';
    case 'native':
      return 'install.step.native';
    case 'asset':
      return 'install.step.asset';
    case 'loader':
      return 'install.step.loader';
    default:
      return 'install.step.jar';
  }
}

// Graphical stage markers for install progress rows (text assists only):
// file-download stages share the download glyph, the loader stage its own.
export function stageIcon(stage: string) {
  if (stage === 'loader') {
    return <Asset id="icon-loader" size={16} />;
  }
  return <Asset id="icon-download" size={16} />;
}
