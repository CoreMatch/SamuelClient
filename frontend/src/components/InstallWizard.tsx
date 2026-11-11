import { useState, useEffect, useCallback, useRef } from 'react';
import { Box, Feather, Flame } from 'lucide-react';
import { Asset } from '@ha/ui';
import {
  GetVersions,
  GetFabricVersions,
  GetForgeVersions,
  DownloadVersion,
  InstallFabricLoader,
  InstallForgeLoader,
  CancelInstall,
} from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { LoadingSkeleton, ErrorState } from './states';
import { t } from '../i18n';
import { Button } from '../ui/button';
import { Badge } from '../ui/badge';
import { Select } from '../ui/select';
import { Field } from '../ui/field';
import { Progress } from '../ui/progress';
import { Callout } from '../ui/callout';
import { RadioCards } from '../ui/radio-cards';

// InstallWizard implements the three-step install flow shared by HMCL/PCL2
// (launcher research 03 §8.3): select version → select loader → confirm and
// watch the progress. It only uses existing, verified bindings and the
// download:progress event contract — no new backend surface.

interface MCVersion {
  id: string;
  type: string;
  url?: string;
  time?: string;
  releaseTime?: string;
}

type LoaderType = 'vanilla' | 'fabric' | 'forge';

interface InstallProgress {
  stage: string;
  pct: number | null;
}

interface InstallWizardProps {
  // Called after a successful install so the installed list refreshes.
  onInstalled: () => void;
}

const REVOLUTION_VERSIONS = ['1.12.2', '1.21.1'];

export default function InstallWizard({ onInstalled }: InstallWizardProps) {
  const [step, setStep] = useState(0);

  // Step 1 state: the version manifest.
  const [versions, setVersions] = useState<MCVersion[]>([]);
  const [loading, setLoading] = useState(false);
  const [fetchError, setFetchError] = useState('');
  const [typeFilter, setTypeFilter] = useState('all');
  const [selectedVersion, setSelectedVersion] = useState<MCVersion | null>(null);

  // Step 2 state: loader choice + available loader versions.
  const [loaderType, setLoaderType] = useState<LoaderType>('vanilla');
  const [fabricVersions, setFabricVersions] = useState<string[]>([]);
  const [forgeVersions, setForgeVersions] = useState<string[]>([]);
  const [loadersLoading, setLoadersLoading] = useState(false);
  const [loadersError, setLoadersError] = useState('');
  const [selectedLoaderVersion, setSelectedLoaderVersion] = useState('');

  // Step 3 state: install lifecycle.
  const [phase, setPhase] = useState<'idle' | 'installing' | 'success' | 'error' | 'cancelled'>('idle');
  const [installError, setInstallError] = useState('');
  const [installedLabel, setInstalledLabel] = useState('');
  const [progress, setProgress] = useState<Record<string, InstallProgress>>({});
  // activeInstallIdRef tracks the version id currently installing so the
  // cancel button targets the right run; cancelRequestedRef distinguishes a
  // user cancel from a genuine failure when the install promise settles.
  const activeInstallIdRef = useRef('');
  const cancelRequestedRef = useRef(false);

  const fetchVersions = useCallback(async () => {
    setLoading(true);
    setFetchError('');
    try {
      const result = await GetVersions();
      if (result.success) {
        if (result.data && Array.isArray(result.data.versions)) {
          setVersions(result.data.versions);
        } else if (Array.isArray(result.data)) {
          setVersions(result.data);
        } else {
          setFetchError('Invalid version data format');
        }
      } else {
        setFetchError(result.error || 'Failed to fetch versions');
      }
    } catch {
      setFetchError('Failed to fetch versions. Please try again.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchVersions();
  }, [fetchVersions]);

  // download:progress events drive the progress row (same contract as the
  // old version list: {version_id, stage, name, downloaded, total}).
  useEffect(() => {
    EventsOn('download:progress', (p: { version_id?: string; stage?: string; downloaded?: number; total?: number }) => {
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

  // Entering the loader step fetches both loader catalogs for the selected
  // Minecraft version (top 5 each, mirroring the previous home dropdown).
  const fetchLoaders = useCallback(async (mcVersion: string) => {
    setLoadersLoading(true);
    setLoadersError('');
    try {
      const [fabricResult, forgeResult] = await Promise.all([
        GetFabricVersions(mcVersion),
        GetForgeVersions(mcVersion),
      ]);
      let failed = '';
      if (fabricResult.success && Array.isArray(fabricResult.data)) {
        setFabricVersions(
          (fabricResult.data as { loader: { version: string } }[]).slice(0, 5).map(f => f.loader.version),
        );
      } else {
        failed = fabricResult.error || 'Failed to fetch Fabric loaders';
      }
      if (forgeResult.success && Array.isArray(forgeResult.data)) {
        setForgeVersions(
          (forgeResult.data as { version: string }[]).slice(0, 5).map(f => f.version),
        );
      } else {
        failed = failed || forgeResult.error || 'Failed to fetch Forge loaders';
      }
      if (failed && !(fabricResult.success || forgeResult.success)) {
        setLoadersError(failed);
      }
    } catch {
      setLoadersError('Failed to fetch loaders. Please try again.');
    } finally {
      setLoadersLoading(false);
    }
  }, []);

  const goNext = () => {
    if (step === 0 && selectedVersion) {
      setStep(1);
      fetchLoaders(selectedVersion.id);
    } else if (step === 1) {
      setStep(2);
    }
  };

  const goBack = () => {
    setStep(s => Math.max(0, s - 1));
  };

  const expectedLoaderVersionId = () => {
    if (!selectedVersion) return '';
    if (loaderType === 'fabric') return `fabric-loader-${selectedLoaderVersion}-${selectedVersion.id}`;
    if (loaderType === 'forge') return `${selectedVersion.id}-forge-${selectedLoaderVersion}`;
    return '';
  };

  const runInstall = async () => {
    if (!selectedVersion) return;
    setPhase('installing');
    setInstallError('');
    cancelRequestedRef.current = false;
    try {
      activeInstallIdRef.current = selectedVersion.id;
      const download = await DownloadVersion(selectedVersion.id, '');
      if (!download.success) {
        throw new Error(download.error || 'Download failed');
      }
      if (loaderType === 'fabric') {
        activeInstallIdRef.current = `fabric-loader-${selectedLoaderVersion}-${selectedVersion.id}`;
        const result = await InstallFabricLoader(selectedVersion.id, selectedLoaderVersion, '');
        if (!result.success) throw new Error(result.error || 'Loader install failed');
        const data = result.data as { version_id?: string } | null;
        setInstalledLabel(data?.version_id || expectedLoaderVersionId());
      } else if (loaderType === 'forge') {
        activeInstallIdRef.current = `${selectedVersion.id}-forge-${selectedLoaderVersion}`;
        const result = await InstallForgeLoader(selectedVersion.id, selectedLoaderVersion, '');
        if (!result.success) throw new Error(result.error || 'Loader install failed');
        const data = result.data as { version_id?: string } | null;
        setInstalledLabel(data?.version_id || expectedLoaderVersionId());
      } else {
        setInstalledLabel(selectedVersion.id);
      }
      setPhase('success');
      onInstalled();
    } catch (err) {
      if (cancelRequestedRef.current) {
        // A user-initiated cancel finalizes cleanly rather than surfacing a
        // failure; the backend already ran its failure cleanup so a retry is
        // safe.
        setPhase('cancelled');
      } else {
        setInstallError(err instanceof Error ? err.message : String(err));
        setPhase('error');
      }
    } finally {
      activeInstallIdRef.current = '';
    }
  };

  const cancelInstall = () => {
    const id = activeInstallIdRef.current;
    if (!id) return;
    cancelRequestedRef.current = true;
    void CancelInstall(id);
  };

  const resetWizard = () => {
    setStep(0);
    setSelectedVersion(null);
    setLoaderType('vanilla');
    setSelectedLoaderVersion('');
    setPhase('idle');
    setInstallError('');
    setInstalledLabel('');
    setProgress({});
  };

  // The progress row follows the active install: the vanilla download first,
  // then the loader install under its own version id.
  const activeProgress: InstallProgress | null = (() => {
    if (phase !== 'installing') return null;
    const loaderId = expectedLoaderVersionId();
    if (loaderId && progress[loaderId]) return progress[loaderId];
    if (selectedVersion && progress[selectedVersion.id]) return progress[selectedVersion.id];
    return null;
  })();

  const filteredVersions = versions.filter(v => typeFilter === 'all' || v.type === typeFilter);

  const stepLabels = [t('wizard.step.version'), t('wizard.step.loader'), t('wizard.step.confirm')];

  return (
    <div>
      {/* Step indicator */}
      <ol className="mb-8 flex items-center gap-4">
        {stepLabels.map((label, i) => (
          <li key={label} className={`flex items-center gap-4 ${i < stepLabels.length - 1 ? 'flex-1' : ''}`}>
            <span className="flex items-center gap-2.5">
              <span
                aria-current={step === i ? 'step' : undefined}
                className={`flex size-8 shrink-0 items-center justify-center rounded-full text-sm font-semibold ${
                  step > i
                    ? 'bg-accent text-on-accent'
                    : step === i
                      ? 'border border-edge-strong bg-white/8 text-ink'
                      : 'border border-edge text-ink-faint'
                }`}
              >
                {step > i ? <Asset id="icon-check" size={16} /> : i + 1}
              </span>
              <span className={`text-sm font-medium ${step >= i ? 'text-ink' : 'text-ink-faint'}`}>{label}</span>
            </span>
            {i < stepLabels.length - 1 && <span className="h-px flex-1 bg-edge" aria-hidden />}
          </li>
        ))}
      </ol>

      {step === 0 && (
        <div>
          <div className="mb-4 flex justify-end">
            <div className="w-52">
              <Select
                ariaLabel={t('versions.filterType')}
                value={typeFilter}
                onValueChange={setTypeFilter}
                options={[
                  { value: 'all', label: t('versions.filter.all') },
                  { value: 'release', label: t('versions.filter.release') },
                  { value: 'snapshot', label: t('versions.filter.snapshot') },
                  { value: 'old_beta', label: t('versions.filter.oldBeta') },
                  { value: 'old_alpha', label: t('versions.filter.oldAlpha') },
                ]}
              />
            </div>
          </div>

          {loading ? (
            <LoadingSkeleton rows={6} />
          ) : fetchError ? (
            <ErrorState data={t('versions.listData')} onRetry={fetchVersions} />
          ) : (
            <div className="max-h-[480px] overflow-y-auto rounded-lg bg-panel">
              {filteredVersions.map((version, index) => (
                <button
                  key={version.id}
                  type="button"
                  onClick={() => setSelectedVersion(version)}
                  aria-pressed={selectedVersion?.id === version.id}
                  className={`flex w-full items-center justify-between gap-3 px-5 py-4 text-left transition-colors duration-150 hover:bg-white/4 ${
                    index > 0 ? 'border-t border-edge' : ''
                  } ${selectedVersion?.id === version.id ? 'bg-white/6' : ''}`}
                >
                  <span className="flex items-center gap-2.5">
                    <Asset id="icon-version" size={20} className="text-ink-muted" />
                    <span className="text-base font-medium text-ink">{version.id}</span>
                  </span>
                  <span className="flex items-center gap-2">
                    <Badge
                      tone={
                        version.type === 'release'
                          ? 'success'
                          : version.type === 'snapshot'
                            ? 'warning'
                            : 'neutral'
                      }
                    >
                      {version.type}
                    </Badge>
                    {REVOLUTION_VERSIONS.includes(version.id) && <Badge tone="info">Revolution</Badge>}
                  </span>
                </button>
              ))}
            </div>
          )}

          <div className="mt-6 flex justify-end">
            <Button onClick={goNext} disabled={!selectedVersion}>
              {t('wizard.next')}
            </Button>
          </div>
        </div>
      )}

      {step === 1 && selectedVersion && (
        <div>
          {loadersLoading && (
            <div className="mb-5">
              <Progress value={null} />
            </div>
          )}
          {loadersError && (
            <div className="mb-5">
              <Callout tone="warning">{loadersError}</Callout>
            </div>
          )}

          <RadioCards
            ariaLabel={t('wizard.step.loader')}
            value={loaderType}
            onValueChange={kind => {
              const next = kind as LoaderType;
              setLoaderType(next);
              setSelectedLoaderVersion(
                next === 'fabric' ? fabricVersions[0] || '' : next === 'forge' ? forgeVersions[0] || '' : '',
              );
            }}
            options={[
              { value: 'vanilla', title: t('wizard.vanilla'), icon: <Box className="size-5" aria-hidden /> },
              { value: 'fabric', title: 'Fabric', icon: <Feather className="size-5" aria-hidden /> },
              { value: 'forge', title: 'Forge', icon: <Flame className="size-5" aria-hidden /> },
            ]}
          />

          {loaderType !== 'vanilla' && (
            <div className="mt-5">
              <Field label={t('wizard.loaderVersion')} htmlFor="wizard-loader-version">
                <Select
                  ariaLabel={t('wizard.loaderVersion')}
                  value={selectedLoaderVersion}
                  onValueChange={setSelectedLoaderVersion}
                  options={(loaderType === 'fabric' ? fabricVersions : forgeVersions).map(v => ({
                    value: v,
                    label: v,
                  }))}
                />
              </Field>
            </div>
          )}

          <div className="mt-8 flex justify-between">
            <Button variant="secondary" onClick={goBack}>
              {t('wizard.back')}
            </Button>
            <Button onClick={goNext} disabled={loaderType !== 'vanilla' && !selectedLoaderVersion}>
              {t('wizard.next')}
            </Button>
          </div>
        </div>
      )}

      {step === 2 && selectedVersion && (
        <div>
          <div className="rounded-lg bg-panel px-6 py-5">
            <p className="text-base text-ink">
              {loaderType === 'vanilla'
                ? t('wizard.summaryVanilla', { version: selectedVersion.id })
                : t('wizard.summary', {
                    version: selectedVersion.id,
                    loader: `${loaderType === 'fabric' ? 'Fabric' : 'Forge'} ${selectedLoaderVersion}`,
                  })}
            </p>
          </div>

          {phase === 'installing' && (
            <div className="mt-5">
              <div className="mb-2 flex items-center justify-between">
                <span className="inline-flex items-center gap-2 text-sm text-ink-muted">
                  {stageIcon(activeProgress?.stage || '')}
                  {t(installStepKey(activeProgress?.stage || ''))}
                </span>
                {activeProgress?.pct != null && (
                  <span className="font-mono text-sm text-ink-muted">{activeProgress.pct}%</span>
                )}
              </div>
              <Progress value={activeProgress?.pct ?? null} />
              <div className="mt-3 flex justify-end">
                <Button variant="secondary" onClick={cancelInstall}>
                  {t('common.cancel')}
                </Button>
              </div>
            </div>
          )}

          {phase === 'success' && (
            <div className="mt-5">
              <Callout tone="success">{t('wizard.installSuccess', { version: installedLabel })}</Callout>
            </div>
          )}

          {phase === 'error' && (
            <div className="mt-5">
              <Callout tone="danger">{t('wizard.installFailed', { error: installError })}</Callout>
            </div>
          )}

          {phase === 'cancelled' && (
            <div className="mt-5">
              <Callout tone="warning">{t('wizard.installCancelled')}</Callout>
            </div>
          )}

          <div className="mt-8 flex justify-between">
            {phase === 'success' ? (
              <Button variant="secondary" onClick={resetWizard}>
                {t('wizard.installAnother')}
              </Button>
            ) : (
              <Button variant="secondary" onClick={goBack} disabled={phase === 'installing'}>
                {t('wizard.back')}
              </Button>
            )}
            {phase === 'success' ? null : phase === 'error' || phase === 'cancelled' ? (
              <Button onClick={runInstall}>{t('common.retry')}</Button>
            ) : (
              <Button onClick={runInstall} disabled={phase === 'installing'} loading={phase === 'installing'}>
                {phase === 'installing' ? t('wizard.installing') : t('wizard.install')}
              </Button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// Maps a "download:progress" stage to its i18n key; unknown stages fall
// back to the generic game-file label instead of rendering a raw key.
function installStepKey(stage: string): 'install.step.jar' | 'install.step.library' | 'install.step.native' | 'install.step.asset' | 'install.step.loader' {
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

// Graphical stage markers for the install progress row (text assists only):
// file-download stages share the download glyph, the loader stage its own.
function stageIcon(stage: string) {
  if (stage === 'loader') {
    return <Asset id="icon-loader" size={16} />;
  }
  return <Asset id="icon-download" size={16} />;
}
