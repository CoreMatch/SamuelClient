import { useState, useEffect } from 'react';
import { FolderOpen, Pencil, Plus, Save, Star, Trash2, X } from 'lucide-react';
import {
  GetLauncherConfig,
  GetJavaInstallations,
  UpdateLauncherConfig,
  AddJavaInstallation,
  UpdateJavaInstallation,
  DeleteJavaInstallation,
  SetDefaultJavaInstallation,
} from '../../wailsjs/go/main/App';
import { LoadingSkeleton } from './states';
import { t } from '../i18n';
import { Button, IconButton } from '../ui/button';
import { TextField } from '../ui/field';
import { Slider } from '../ui/progress';
import { Callout } from '../ui/callout';
import { Dialog, AlertDialog } from '../ui/dialog';
import { Tooltip } from '../ui/tooltip';

interface JavaInstallation {
  id: number;
  path: string;
  friendly_name: string;
  version: string;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

interface LauncherConfig {
  java_path: string;
  game_dir: string;
  max_memory: number;
  min_memory: number;
  game_width: number;
  game_height: number;
  java_args?: string;
}

// Maximum memory cap for the slider. Minecraft's launcher standard does not
// cap this explicitly, but 32 GB is well past any practical gameplay need
// and keeps the slider responsive.
const MAX_MEMORY_MB = 32768;
const MIN_MEMORY_MB = 256;
const MAX_GAME_DIMENSION = 3840;
const MIN_GAME_DIMENSION = 320;

const sectionClasses = 'rounded-lg bg-panel p-8';

export default function SettingsView() {
  const [config, setConfig] = useState<LauncherConfig | null>(null);
  const [javaInstallations, setJavaInstallations] = useState<JavaInstallation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [openDialog, setOpenDialog] = useState(false);
  const [editingJava, setEditingJava] = useState<JavaInstallation | null>(null);
  const [newJavaPath, setNewJavaPath] = useState('');
  const [newJavaName, setNewJavaName] = useState('');
  const [dialogError, setDialogError] = useState('');
  const [deleteJavaId, setDeleteJavaId] = useState<number | null>(null);

  // Memory settings draft (the values shown in inputs/sliders before saving).
  // Kept separate from `config` so the user can tweak without losing the
  // last-saved value if validation fails.
  const [minMemory, setMinMemory] = useState(1024);
  const [maxMemory, setMaxMemory] = useState(4096);
  const [javaArgs, setJavaArgs] = useState('');
  const [gameWidth, setGameWidth] = useState(854);
  const [gameHeight, setGameHeight] = useState(480);
  const [savingConfig, setSavingConfig] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    fetchConfig();
    fetchJavaInstallations();
  }, []);

  const fetchConfig = async () => {
    try {
      const result = await GetLauncherConfig();
      if (result.success && result.data) {
        setConfig(result.data);
        setMinMemory(result.data.min_memory ?? 1024);
        setMaxMemory(result.data.max_memory ?? 4096);
        setJavaArgs(result.data.java_args ?? '');
        setGameWidth(result.data.game_width ?? 854);
        setGameHeight(result.data.game_height ?? 480);
      }
    } catch (err) {
      console.error('Failed to fetch config:', err);
    }
  };

  const fetchJavaInstallations = async () => {
    setLoading(true);
    setError('');
    try {
      const result = await GetJavaInstallations();
      if (result.success && Array.isArray(result.data)) {
        setJavaInstallations(result.data);
      } else if (result.error) {
        setError(result.error);
      }
    } catch {
      setError(t('settings.javaFetchFailed'));
    } finally {
      setLoading(false);
    }
  };

  // validateMemorySettings returns the first user-facing validation error, or
  // empty string when the in-progress memory/JVM values are acceptable.
  // Bounds align with the backend SaveLauncherConfig validator and the
  // slider caps defined above.
  const validateMemorySettings = (): string => {
    if (minMemory < MIN_MEMORY_MB) {
      return t('settings.err.minMemoryMin', { min: MIN_MEMORY_MB });
    }
    if (maxMemory < MIN_MEMORY_MB) {
      return t('settings.err.maxMemoryMin', { min: MIN_MEMORY_MB });
    }
    if (maxMemory > MAX_MEMORY_MB) {
      return t('settings.err.maxMemoryMax', { max: MAX_MEMORY_MB });
    }
    if (minMemory > maxMemory) {
      return t('settings.err.minGtMax');
    }
    if (gameWidth < MIN_GAME_DIMENSION || gameWidth > MAX_GAME_DIMENSION) {
      return t('settings.err.widthRange', { min: MIN_GAME_DIMENSION, max: MAX_GAME_DIMENSION });
    }
    if (gameHeight < MIN_GAME_DIMENSION || gameHeight > MAX_GAME_DIMENSION) {
      return t('settings.err.heightRange', { min: MIN_GAME_DIMENSION, max: MAX_GAME_DIMENSION });
    }
    return '';
  };

  // handleSaveConfig persists the in-progress memory settings through the
  // UpdateLauncherConfig Wails binding. On success it replaces `config`
  // with the merged response from the backend and clears the dirty flag.
  const handleSaveConfig = async () => {
    const validationError = validateMemorySettings();
    if (validationError) {
      setSaveError(validationError);
      return;
    }

    setSavingConfig(true);
    setSaveError('');
    try {
      // ADR-0004: config writes go through the in-process Wails binding. The
      // auto-detected fields (java_path, game_dir, authlib_jar) are sent empty
      // so SaveLauncherConfig leaves them untouched, matching the old PUT body.
      const result = await UpdateLauncherConfig({
        java_path: '',
        max_memory: maxMemory,
        min_memory: minMemory,
        java_args: javaArgs,
        game_width: gameWidth,
        game_height: gameHeight,
        game_dir: '',
        java_dir: '',
        bmclapi_mirrors: '',
        authlib_jar: '',
      });
      if (result.success && result.data) {
        setConfig(result.data);
        setMinMemory(result.data.min_memory ?? minMemory);
        setMaxMemory(result.data.max_memory ?? maxMemory);
        setJavaArgs(result.data.java_args ?? '');
        setGameWidth(result.data.game_width ?? gameWidth);
        setGameHeight(result.data.game_height ?? gameHeight);
        setDirty(false);
        setSaveSuccess(true);
        setTimeout(() => setSaveSuccess(false), 3000);
      } else {
        setSaveError(result.error || t('settings.failedSave'));
      }
    } catch {
      setSaveError(t('settings.failedSave'));
    } finally {
      setSavingConfig(false);
    }
  };

  const handleResetConfig = () => {
    if (!config) return;
    setMinMemory(config.min_memory ?? 1024);
    setMaxMemory(config.max_memory ?? 4096);
    setJavaArgs(config.java_args ?? '');
    setGameWidth(config.game_width ?? 854);
    setGameHeight(config.game_height ?? 480);
    setDirty(false);
    setSaveError('');
  };

  const handleAddJava = async () => {
    setDialogError('');
    if (!newJavaPath.trim()) {
      setDialogError(t('settings.javaPathRequired'));
      return;
    }

    try {
      const result = await AddJavaInstallation(newJavaPath.trim(), newJavaName.trim());
      if (result.success) {
        fetchJavaInstallations();
        setOpenDialog(false);
        setNewJavaPath('');
        setNewJavaName('');
      } else {
        setDialogError(result.error || t('settings.javaAddFailed'));
      }
    } catch {
      setDialogError(t('settings.javaAddFailed'));
    }
  };

  const handleUpdateJava = async () => {
    setDialogError('');
    if (!editingJava) return;

    try {
      const result = await UpdateJavaInstallation(
        editingJava.id,
        newJavaPath.trim() || editingJava.path,
        newJavaName.trim()
      );
      if (result.success) {
        fetchJavaInstallations();
        setOpenDialog(false);
        setEditingJava(null);
        setNewJavaPath('');
        setNewJavaName('');
      } else {
        setDialogError(result.error || t('settings.javaUpdateFailed'));
      }
    } catch {
      setDialogError(t('settings.javaUpdateFailed'));
    }
  };

  const handleDeleteJava = async (id: number) => {
    try {
      const result = await DeleteJavaInstallation(id);
      if (result.success) {
        fetchJavaInstallations();
      } else {
        setError(result.error || t('settings.javaDeleteFailed'));
      }
    } catch {
      setError(t('settings.javaDeleteFailed'));
    }
  };

  const handleSetDefault = async (id: number) => {
    try {
      const result = await SetDefaultJavaInstallation(id);
      if (result.success) {
        fetchJavaInstallations();
      } else {
        setError(result.error || t('settings.javaDefaultFailed'));
      }
    } catch {
      setError(t('settings.javaDefaultFailed'));
    }
  };

  const openAddDialog = () => {
    setEditingJava(null);
    setNewJavaPath('');
    setNewJavaName('');
    setDialogError('');
    setOpenDialog(true);
  };

  const openEditDialog = (java: JavaInstallation) => {
    setEditingJava(java);
    setNewJavaPath(java.path);
    setNewJavaName(java.friendly_name);
    setDialogError('');
    setOpenDialog(true);
  };

  if (loading) {
    return <LoadingSkeleton rows={4} />;
  }

  const defaultJava = javaInstallations.find(j => j.is_default);

  return (
    <div className="flex flex-col gap-8">
      {error && (
        <Callout tone="danger" onDismiss={() => setError('')}>
          {error}
        </Callout>
      )}

      {/* Java installations */}
      <section className={sectionClasses}>
        <div className="mb-5 flex items-center justify-between">
          <h2 className="text-xl font-semibold text-ink">{t('settings.java')}</h2>
          <Button onClick={openAddDialog}>
            <Plus className="size-4" aria-hidden />
            {t('settings.addJava')}
          </Button>
        </div>

        {javaInstallations.length === 0 ? (
          <p className="text-sm text-ink-muted">{t('settings.noJava')}</p>
        ) : (
          <div className="flex flex-col gap-4">
            {javaInstallations.map(java => (
              <div key={java.id} className="rounded-md bg-raised p-5">
                <div className="flex items-center gap-3">
                  <Tooltip label={java.is_default ? t('settings.defaultJava') : t('settings.setDefault')}>
                    <IconButton
                      label={java.is_default ? t('settings.defaultJava') : t('settings.setDefault')}
                      onClick={() => handleSetDefault(java.id)}
                    >
                      <Star
                        className={`size-5 ${java.is_default ? 'fill-warn text-warn' : ''}`}
                        aria-hidden
                      />
                    </IconButton>
                  </Tooltip>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-base font-medium text-ink">
                      {java.friendly_name || `Java ${java.version}`}
                    </p>
                    <p className="font-mono text-sm text-ink-faint">{java.version}</p>
                  </div>
                  <Tooltip label={t('settings.edit')}>
                    <IconButton label={t('settings.edit')} onClick={() => openEditDialog(java)}>
                      <Pencil className="size-5" />
                    </IconButton>
                  </Tooltip>
                  <Tooltip label={t('common.delete')}>
                    <IconButton label={t('common.delete')} tone="danger" onClick={() => setDeleteJavaId(java.id)}>
                      <Trash2 className="size-5" />
                    </IconButton>
                  </Tooltip>
                </div>
                <p className="mt-3 break-all rounded bg-white/4 px-3 py-1.5 font-mono text-sm text-ink-muted">
                  {java.path}
                </p>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* Paths */}
      <section className={sectionClasses}>
        <h2 className="mb-5 text-xl font-semibold text-ink">{t('settings.paths')}</h2>
        <div className="flex flex-col gap-6">
          <div>
            <p className="text-sm font-medium text-ink-muted">{t('settings.javaPath')}</p>
            {defaultJava ? (
              <div className="mt-2">
                <p className="text-base font-medium text-ink">
                  {defaultJava.friendly_name || `Java ${defaultJava.version}`}
                </p>
                <p className="mt-1.5 break-all rounded bg-white/4 px-3 py-1.5 font-mono text-sm text-ink-muted">
                  {defaultJava.path}
                </p>
              </div>
            ) : (
              <p className="mt-2 break-all rounded bg-white/4 px-3 py-1.5 font-mono text-sm text-ink-muted">
                {config?.java_path || t('settings.notConfigured')}
              </p>
            )}
          </div>
          <div>
            <p className="text-sm font-medium text-ink-muted">{t('settings.gameDir')}</p>
            <p className="mt-2 break-all rounded bg-white/4 px-3 py-1.5 font-mono text-sm text-ink-muted">
              {config?.game_dir || t('settings.notConfigured')}
            </p>
          </div>
        </div>
      </section>

      {/* Folders */}
      <section className={sectionClasses}>
        <h2 className="mb-5 text-xl font-semibold text-ink">{t('settings.folders')}</h2>
        <Button variant="secondary" onClick={() => window.open('file:///home/lnb/.minecraft/mods', '_blank')}>
          <FolderOpen className="size-5" aria-hidden />
          {t('settings.modsFolder')}
        </Button>
      </section>

      {/* Memory & JVM */}
      <section className={sectionClasses}>
        <div className="mb-5 flex flex-wrap items-center justify-between gap-4">
          <h2 className="text-xl font-semibold text-ink">{t('settings.memory')}</h2>
          <div className="flex items-center gap-3">
            {saveSuccess && <span className="text-sm text-ok">{t('settings.saved')}</span>}
            <Button variant="secondary" onClick={handleResetConfig} disabled={!dirty || savingConfig}>
              <X className="size-4" aria-hidden />
              {t('settings.reset')}
            </Button>
            <Button onClick={handleSaveConfig} disabled={!dirty || savingConfig} loading={savingConfig}>
              {!savingConfig && <Save className="size-4" aria-hidden />}
              {savingConfig ? t('settings.saving') : t('settings.save')}
            </Button>
          </div>
        </div>

        {saveError && (
          <div className="mb-5">
            <Callout tone="danger" onDismiss={() => setSaveError('')}>
              {saveError}
            </Callout>
          </div>
        )}

        <div className="flex flex-col gap-7">
          <div>
            <div className="mb-3 flex items-baseline justify-between">
              <span className="text-sm text-ink-muted">{t('settings.maxMemory')}</span>
              <span className="font-mono text-base text-ink">{maxMemory} MB</span>
            </div>
            <Slider
              ariaLabel={t('settings.maxMemory')}
              value={maxMemory}
              min={MIN_MEMORY_MB}
              max={MAX_MEMORY_MB}
              step={256}
              onValueChange={v => {
                setMaxMemory(v);
                setDirty(true);
              }}
            />
            <input
              type="number"
              aria-label={t('settings.maxMemory')}
              value={maxMemory}
              min={MIN_MEMORY_MB}
              max={MAX_MEMORY_MB}
              step={256}
              onChange={e => {
                const v = Number(e.target.value);
                if (!Number.isNaN(v)) {
                  setMaxMemory(v);
                  setDirty(true);
                }
              }}
              className="mt-4 h-11 w-64 rounded-md border border-edge bg-raised px-4 font-mono text-sm text-ink transition-colors duration-150 hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong"
            />
          </div>

          <div>
            <div className="mb-3 flex items-baseline justify-between">
              <span className="text-sm text-ink-muted">{t('settings.minMemory')}</span>
              <span className="font-mono text-base text-ink">{minMemory} MB</span>
            </div>
            <Slider
              ariaLabel={t('settings.minMemory')}
              value={Math.min(minMemory, maxMemory)}
              min={MIN_MEMORY_MB}
              max={maxMemory}
              step={256}
              onValueChange={v => {
                setMinMemory(v);
                setDirty(true);
              }}
            />
            <input
              type="number"
              aria-label={t('settings.minMemory')}
              value={minMemory}
              min={MIN_MEMORY_MB}
              max={MAX_MEMORY_MB}
              step={256}
              onChange={e => {
                const v = Number(e.target.value);
                if (!Number.isNaN(v)) {
                  setMinMemory(v);
                  setDirty(true);
                }
              }}
              className="mt-4 h-11 w-64 rounded-md border border-edge bg-raised px-4 font-mono text-sm text-ink transition-colors duration-150 hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong"
            />
          </div>

          <TextField
            label={t('settings.javaArgsLabel')}
            value={javaArgs}
            onChange={e => {
              setJavaArgs(e.target.value);
              setDirty(true);
            }}
            placeholder="-XX:+UseG1GC"
          />

          <div>
            <p className="mb-2 text-sm font-medium text-ink-muted">{t('settings.windowSize')}</p>
            <div className="flex gap-4">
              <input
                type="number"
                aria-label={t('play.width')}
                value={gameWidth}
                min={MIN_GAME_DIMENSION}
                max={MAX_GAME_DIMENSION}
                onChange={e => {
                  const v = Number(e.target.value);
                  if (!Number.isNaN(v)) {
                    setGameWidth(v);
                    setDirty(true);
                  }
                }}
                className="h-11 w-44 rounded-md border border-edge bg-raised px-4 font-mono text-sm text-ink transition-colors duration-150 hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong"
              />
              <input
                type="number"
                aria-label={t('play.height')}
                value={gameHeight}
                min={MIN_GAME_DIMENSION}
                max={MAX_GAME_DIMENSION}
                onChange={e => {
                  const v = Number(e.target.value);
                  if (!Number.isNaN(v)) {
                    setGameHeight(v);
                    setDirty(true);
                  }
                }}
                className="h-11 w-44 rounded-md border border-edge bg-raised px-4 font-mono text-sm text-ink transition-colors duration-150 hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong"
              />
            </div>
          </div>

          <div>
            <p className="mb-2 text-sm font-medium text-ink-muted">{t('settings.jvmPreview')}</p>
            <p className="break-all rounded bg-white/4 px-4 py-2.5 font-mono text-sm text-ink-muted">
              {`-Xms${minMemory}m -Xmx${maxMemory}m ` +
                (javaArgs.trim() ? `${javaArgs.trim()} ` : '') +
                `--width ${gameWidth} --height ${gameHeight}`}
            </p>
          </div>
        </div>
      </section>

      {/* Add / edit Java dialog */}
      <Dialog
        open={openDialog}
        onOpenChange={open => {
          setOpenDialog(open);
          if (!open) {
            setEditingJava(null);
            setNewJavaPath('');
            setNewJavaName('');
            setDialogError('');
          }
        }}
        title={editingJava ? t('settings.editJavaTitle') : t('settings.addJavaTitle')}
        closeLabel={t('common.close')}
      >
        <div className="flex flex-col gap-5">
          <TextField
            label={t('settings.javaPath')}
            value={newJavaPath}
            onChange={e => setNewJavaPath(e.target.value)}
            placeholder="/usr/bin/java"
          />
          <TextField
            label={t('settings.friendlyName')}
            value={newJavaName}
            onChange={e => setNewJavaName(e.target.value)}
            placeholder="Java 17"
          />
          {dialogError && <Callout tone="danger">{dialogError}</Callout>}
          <div className="mt-3 flex justify-end gap-3">
            <Button variant="secondary" onClick={() => setOpenDialog(false)}>
              {t('common.cancel')}
            </Button>
            <Button onClick={editingJava ? handleUpdateJava : handleAddJava}>
              {editingJava ? t('settings.update') : t('settings.addJava')}
            </Button>
          </div>
        </div>
      </Dialog>

      {/* Delete Java confirmation */}
      <AlertDialog
        open={deleteJavaId !== null}
        onOpenChange={open => {
          if (!open) setDeleteJavaId(null);
        }}
        title={t('settings.deleteJavaTitle')}
        description={t('settings.deleteJavaConfirm')}
        confirmLabel={t('common.delete')}
        cancelLabel={t('common.cancel')}
        onConfirm={() => {
          if (deleteJavaId !== null) {
            handleDeleteJava(deleteJavaId);
          }
          setDeleteJavaId(null);
        }}
      />
    </div>
  );
}
