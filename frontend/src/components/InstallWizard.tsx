import { useState, useEffect, useCallback } from 'react';
import {
  Alert,
  Box,
  Button,
  Chip,
  FormControl,
  InputLabel,
  LinearProgress,
  List,
  ListItemButton,
  ListItemText,
  MenuItem,
  Select,
  Step,
  StepLabel,
  Stepper,
  Typography,
} from '@mui/material';
import {
  GetVersions,
  GetFabricVersions,
  GetForgeVersions,
  DownloadVersion,
  InstallFabricLoader,
  InstallForgeLoader,
} from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { LoadingSkeleton, ErrorState } from './states';
import { t } from '../i18n';
import { tokens } from '../theme';

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
  const [phase, setPhase] = useState<'idle' | 'installing' | 'success' | 'error'>('idle');
  const [installError, setInstallError] = useState('');
  const [installedLabel, setInstalledLabel] = useState('');
  const [progress, setProgress] = useState<Record<string, InstallProgress>>({});

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
    try {
      const download = await DownloadVersion(selectedVersion.id, '');
      if (!download.success) {
        throw new Error(download.error || 'Download failed');
      }
      if (loaderType === 'fabric') {
        const result = await InstallFabricLoader(selectedVersion.id, selectedLoaderVersion, '');
        if (!result.success) throw new Error(result.error || 'Loader install failed');
        const data = result.data as { version_id?: string } | null;
        setInstalledLabel(data?.version_id || expectedLoaderVersionId());
      } else if (loaderType === 'forge') {
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
      setInstallError(err instanceof Error ? err.message : String(err));
      setPhase('error');
    }
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

  return (
    <Box>
      <Stepper activeStep={step} sx={{ mb: 3 }}>
        {[t('wizard.step.version'), t('wizard.step.loader'), t('wizard.step.confirm')].map(label => (
          <Step key={label}>
            <StepLabel>{label}</StepLabel>
          </Step>
        ))}
      </Stepper>

      {step === 0 && (
        <Box>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2, gap: 2, flexWrap: 'wrap' }}>
            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
              {t('versions.recommended', { versions: REVOLUTION_VERSIONS.join(', ') })}
            </Typography>
            <FormControl size="small" sx={{ minWidth: 160 }}>
              <InputLabel>{t('versions.filterType')}</InputLabel>
              <Select
                value={typeFilter}
                label={t('versions.filterType')}
                onChange={(e) => setTypeFilter(e.target.value)}
              >
                <MenuItem value="all">All</MenuItem>
                <MenuItem value="release">Release</MenuItem>
                <MenuItem value="snapshot">Snapshot</MenuItem>
                <MenuItem value="old_beta">Old Beta</MenuItem>
                <MenuItem value="old_alpha">Old Alpha</MenuItem>
              </Select>
            </FormControl>
          </Box>

          {loading ? (
            <LoadingSkeleton rows={6} />
          ) : fetchError ? (
            <ErrorState data={t('versions.listData')} onRetry={fetchVersions} />
          ) : (
            <List
              sx={{
                backgroundColor: 'background.paper',
                borderRadius: tokens.radius.lg,
                maxHeight: 420,
                overflowY: 'auto',
              }}
            >
              {filteredVersions.map(version => (
                <ListItemButton
                  key={version.id}
                  selected={selectedVersion?.id === version.id}
                  onClick={() => setSelectedVersion(version)}
                  sx={{
                    borderRadius: tokens.radius.md,
                    '&.Mui-selected': { backgroundColor: 'rgba(233, 69, 96, 0.16)' },
                  }}
                >
                  <ListItemText
                    primary={<Typography sx={{ color: 'text.primary' }}>{version.id}</Typography>}
                    secondary={
                      <Box sx={{ display: 'flex', gap: 1, alignItems: 'center', mt: 0.5 }}>
                        <Chip
                          size="small"
                          label={version.type}
                          sx={{
                            height: 20,
                            fontSize: '0.7rem',
                            backgroundColor:
                              version.type === 'release'
                                ? tokens.color.success
                                : version.type === 'snapshot'
                                  ? tokens.color.warning
                                  : 'rgba(255, 255, 255, 0.24)',
                            color: tokens.color.deep,
                          }}
                        />
                        {REVOLUTION_VERSIONS.includes(version.id) && (
                          <Chip
                            size="small"
                            label="Revolution"
                            sx={{ height: 20, fontSize: '0.7rem', backgroundColor: tokens.color.info, color: tokens.color.deep }}
                          />
                        )}
                      </Box>
                    }
                  />
                </ListItemButton>
              ))}
            </List>
          )}

          <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 2 }}>
            <Button variant="contained" onClick={goNext} disabled={!selectedVersion}>
              {t('wizard.next')}
            </Button>
          </Box>
        </Box>
      )}

      {step === 1 && selectedVersion && (
        <Box>
          {loadersLoading && <LinearProgress sx={{ mb: 2, borderRadius: 3 }} />}
          {loadersError && (
            <Alert severity="warning" sx={{ mb: 2 }}>
              {loadersError}
            </Alert>
          )}

          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5, mb: 2 }}>
            {(['vanilla', 'fabric', 'forge'] as LoaderType[]).map(kind => (
              <Box
                key={kind}
                onClick={() => {
                  setLoaderType(kind);
                  setSelectedLoaderVersion(kind === 'fabric' ? fabricVersions[0] || '' : kind === 'forge' ? forgeVersions[0] || '' : '');
                }}
                sx={{
                  p: 2,
                  borderRadius: tokens.radius.lg,
                  cursor: 'pointer',
                  backgroundColor: 'background.paper',
                  border: loaderType === kind ? `2px solid ${tokens.color.accent}` : `2px solid ${tokens.color.divider}`,
                }}
              >
                <Typography sx={{ color: 'text.primary', fontWeight: 600 }}>
                  {kind === 'vanilla' ? t('wizard.vanilla') : kind === 'fabric' ? 'Fabric' : 'Forge'}
                </Typography>
                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                  {kind === 'vanilla'
                    ? t('wizard.vanillaHint')
                    : kind === 'fabric'
                      ? t('wizard.fabricHint')
                      : t('wizard.forgeHint')}
                </Typography>
                {kind !== 'vanilla' && !loadersLoading && (
                  <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {(kind === 'fabric' ? fabricVersions : forgeVersions).length} loader versions available
                  </Typography>
                )}
              </Box>
            ))}
          </Box>

          {loaderType !== 'vanilla' && (
            <FormControl fullWidth size="small" sx={{ mb: 2 }}>
              <InputLabel>Loader version</InputLabel>
              <Select
                value={selectedLoaderVersion}
                label="Loader version"
                onChange={(e) => setSelectedLoaderVersion(e.target.value)}
              >
                {(loaderType === 'fabric' ? fabricVersions : forgeVersions).map(v => (
                  <MenuItem key={v} value={v}>
                    {v}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          )}

          <Box sx={{ display: 'flex', justifyContent: 'space-between', mt: 2 }}>
            <Button variant="outlined" onClick={goBack}>
              {t('wizard.back')}
            </Button>
            <Button
              variant="contained"
              onClick={goNext}
              disabled={loaderType !== 'vanilla' && !selectedLoaderVersion}
            >
              {t('wizard.next')}
            </Button>
          </Box>
        </Box>
      )}

      {step === 2 && selectedVersion && (
        <Box>
          <Box sx={{ p: 2, mb: 2, backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
            <Typography sx={{ color: 'text.primary' }}>
              {loaderType === 'vanilla'
                ? t('wizard.summaryVanilla', { version: selectedVersion.id })
                : t('wizard.summary', {
                    version: selectedVersion.id,
                    loader: `${loaderType === 'fabric' ? 'Fabric' : 'Forge'} ${selectedLoaderVersion}`,
                  })}
            </Typography>
          </Box>

          {phase === 'installing' && (
            <Box sx={{ mb: 2 }}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                  {t(installStepKey(activeProgress?.stage || ''))}
                </Typography>
                {activeProgress?.pct != null && (
                  <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {activeProgress.pct}%
                  </Typography>
                )}
              </Box>
              <LinearProgress
                variant={activeProgress?.pct == null ? 'indeterminate' : 'determinate'}
                value={activeProgress?.pct ?? 0}
                sx={{
                  height: 8,
                  borderRadius: 4,
                  backgroundColor: 'rgba(255, 255, 255, 0.12)',
                  '& .MuiLinearProgress-bar': { backgroundColor: tokens.color.accent, borderRadius: 4 },
                }}
              />
            </Box>
          )}

          {phase === 'success' && (
            <Alert severity="success" sx={{ mb: 2 }}>
              {t('wizard.installSuccess', { version: installedLabel })}
            </Alert>
          )}

          {phase === 'error' && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {t('wizard.installFailed', { error: installError })}
            </Alert>
          )}

          <Box sx={{ display: 'flex', justifyContent: 'space-between', mt: 2 }}>
            {phase === 'success' ? (
              <Button variant="outlined" onClick={resetWizard}>
                {t('wizard.installAnother')}
              </Button>
            ) : (
              <Button variant="outlined" onClick={goBack} disabled={phase === 'installing'}>
                {t('wizard.back')}
              </Button>
            )}
            {phase === 'success' ? null : phase === 'error' ? (
              <Button variant="contained" onClick={runInstall}>
                {t('common.retry')}
              </Button>
            ) : (
              <Button variant="contained" onClick={runInstall} disabled={phase === 'installing'}>
                {phase === 'installing' ? t('wizard.installing') : t('wizard.install')}
              </Button>
            )}
          </Box>
        </Box>
      )}
    </Box>
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
