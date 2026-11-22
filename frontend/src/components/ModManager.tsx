import { useEffect, useState } from 'react';
import { Plus, RefreshCw } from 'lucide-react';
import { AddMod, DeleteMod, ListMods, SetModEnabled } from '../../wailsjs/go/main/App';
import { t } from '../i18n';
import { Badge } from '../ui/badge';
import { Button } from '../ui/button';
import { Callout } from '../ui/callout';
import { AlertDialog } from '../ui/dialog';
import { Select } from '../ui/select';

interface ModFile {
  name: string;
  enabled: boolean;
}

async function readMods(gameDir: string): Promise<ModFile[]> {
  const result = await ListMods(gameDir);
  if (!result.success) throw new Error(result.error || t('mods.failed'));
  return result.data as ModFile[];
}

export default function ModManager({ gameDirs, defaultGameDir }: {
  gameDirs: string[];
  defaultGameDir: string;
}) {
  const [selected, setSelected] = useState('');
  const [busy, setBusy] = useState(false);
  const gameDir = gameDirs.includes(selected) ? selected : defaultGameDir;
  const options = [...new Set([defaultGameDir, ...gameDirs])];

  return (
    <div className="mt-5 flex flex-col gap-4 border-t border-edge pt-5">
      <Select
        ariaLabel={t('settings.gameDir')}
        value={gameDir}
        options={options.map(path => ({ value: path, label: path }))}
        disabled={busy}
        onValueChange={setSelected}
      />
      <ModList key={gameDir} gameDir={gameDir} busy={busy} setBusy={setBusy} />
    </div>
  );
}

function ModList({ gameDir, busy, setBusy }: {
  gameDir: string;
  busy: boolean;
  setBusy: (value: boolean) => void;
}) {
  const [mods, setMods] = useState<ModFile[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [pendingDelete, setPendingDelete] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    void readMods(gameDir).then(list => {
      if (active) setMods(list);
    }).catch(err => {
      if (active) setError(err instanceof Error ? err.message : t('mods.failed'));
    }).finally(() => {
      if (active) setLoading(false);
    });
    return () => { active = false; };
  }, [gameDir]);

  async function run(action?: () => ReturnType<typeof AddMod>) {
    setBusy(true);
    setError('');
    try {
      if (action) {
        const result = await action();
        if (!result.success) throw new Error(result.error || t('mods.failed'));
        if (result.data === false) return; // Cancelled file picker.
      }
      setMods(await readMods(gameDir));
    } catch (err) {
      setError(err instanceof Error ? err.message : t('mods.failed'));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <div className="flex flex-wrap gap-3">
        <Button onClick={() => void run(() => AddMod(gameDir))} disabled={loading || busy}>
          <Plus className="size-4" aria-hidden />
          {t('mods.add')}
        </Button>
        <Button variant="secondary" onClick={() => void run()} disabled={loading || busy}>
          <RefreshCw className="size-4" aria-hidden />
          {t('mods.refresh')}
        </Button>
      </div>
      {error && <Callout tone="danger" onDismiss={() => setError('')}>{error}</Callout>}
      {loading && <p role="status" className="text-sm text-ink-muted">{t('mods.loading')}</p>}
      {mods?.length === 0 && <p className="text-sm text-ink-muted">{t('mods.empty')}</p>}
      {mods && mods.length > 0 && (
        <ul aria-label={t('mods.manage')} className="flex flex-col gap-2" aria-busy={busy}>
          {mods.map(mod => (
            <li key={mod.name} className="flex flex-wrap items-center justify-between gap-3 rounded-md bg-raised px-4 py-3">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <span className="break-all font-mono text-sm text-ink">{mod.name}</span>
                <Badge tone={mod.enabled ? 'success' : 'neutral'}>
                  {t(mod.enabled ? 'mods.enabled' : 'mods.disabled')}
                </Badge>
              </div>
              <div className="flex gap-2">
                <Button
                  variant="secondary"
                  disabled={busy}
                  aria-label={t(mod.enabled ? 'mods.disableFile' : 'mods.enableFile', { name: mod.name })}
                  onClick={() => void run(() => SetModEnabled(gameDir, mod.name, !mod.enabled))}
                >
                  {t(mod.enabled ? 'mods.disable' : 'mods.enable')}
                </Button>
                <Button
                  variant="secondary"
                  disabled={busy}
                  aria-label={t('mods.deleteFile', { name: mod.name })}
                  onClick={() => setPendingDelete(mod.name)}
                >
                  {t('common.delete')}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      <AlertDialog
        open={pendingDelete !== null}
        onOpenChange={open => { if (!open) setPendingDelete(null); }}
        title={t('mods.deleteTitle')}
        description={t('mods.deleteConfirm', { name: pendingDelete ?? '' })}
        confirmLabel={t('common.delete')}
        cancelLabel={t('common.cancel')}
        onConfirm={() => {
          if (pendingDelete) void run(() => DeleteMod(gameDir, pendingDelete));
        }}
      />
    </>
  );
}
