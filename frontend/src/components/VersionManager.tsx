import { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Button,
  Chip,
  LinearProgress,
  Alert,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Tabs,
  Tab,
} from '@mui/material';
import {
  Download,
  Refresh,
  CheckCircle,
  CloudDownload,
  Delete,
} from '@mui/icons-material';
import { GetVersions, DownloadVersion } from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';
import { InstalledVersion } from '../types';
import { LoadingSkeleton, ErrorState, EmptyState } from './states';
import { t } from '../i18n';
import { tokens } from '../theme';

interface MCVersion {
  id: string;
  type: string;
  url?: string;
  time?: string;
  releaseTime?: string;
}

interface VersionManagerProps {
  onSelectVersion: (version: InstalledVersion) => void;
}

const STORAGE_KEY = 'installed_versions';

const REVOLUTION_VERSIONS = ['1.12.2', '1.21.1'];

// InstallProgress is the readable form of one "download:progress" event
// (stage hierarchy per INTEROP/binding contract: jar → library → native →
// asset, plus loader installs).
interface InstallProgress {
  stage: string;
  pct: number | null;
}

export default function VersionManager({ onSelectVersion }: VersionManagerProps) {
  const [versions, setVersions] = useState<MCVersion[]>([]);
  const [installedVersions, setInstalledVersions] = useState<InstalledVersion[]>([]);
  const [loading, setLoading] = useState(false);
  const [downloading, setDownloading] = useState<string | null>(null);
  const [downloadProgress, setDownloadProgress] = useState<Record<string, InstallProgress>>({});
  const [fetchError, setFetchError] = useState('');
  const [downloadError, setDownloadError] = useState('');
  const [tabValue, setTabValue] = useState(0);
  const [typeFilter, setTypeFilter] = useState<string>('all');

  useEffect(() => {
    loadInstalledVersions();
    fetchVersions();
  }, []);

  // ADR-0004: download progress is pushed by the backend as
  // "download:progress" events ({version_id, stage, name, downloaded,
  // total}); subscribe once and unsubscribe on unmount. No HTTP polling.
  useEffect(() => {
    EventsOn('download:progress', (p: { version_id?: string; stage?: string; downloaded?: number; total?: number }) => {
      if (!p || !p.version_id) return;
      let pct: number | null = null;
      if (p.total && p.total > 0 && typeof p.downloaded === 'number') {
        pct = Math.min(100, Math.floor((p.downloaded / p.total) * 100));
      }
      setDownloadProgress(prev => ({ ...prev, [p.version_id as string]: { stage: p.stage || '', pct } }));
    });
    return () => {
      EventsOff('download:progress');
    };
  }, []);

  const loadInstalledVersions = () => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        setInstalledVersions(JSON.parse(stored));
      }
    } catch (err) {
      console.error('Failed to load installed versions:', err);
    }
  };

  const saveInstalledVersions = (versions: InstalledVersion[]) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(versions));
      setInstalledVersions(versions);
    } catch (err) {
      console.error('Failed to save installed versions:', err);
    }
  };

  const fetchVersions = async () => {
    setLoading(true);
    setFetchError('');
    try {
      const result = await GetVersions();
      if (result.success) {
        // Handle BMCLAPI version manifest response format
        // The response has { latest: {...}, versions: [...] }
        if (result.data && Array.isArray(result.data.versions)) {
          setVersions(result.data.versions);
        } else if (Array.isArray(result.data)) {
          // Fallback for old format
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
  };

  const handleDownload = async (version: MCVersion) => {
    setDownloading(version.id);
    setDownloadError('');
    try {
      // ADR-0004: install runs through the in-process Wails binding; progress
      // arrives over "download:progress" events (see subscription above).
      const result = await DownloadVersion(version.id, '');
      if (result.success) {
        // Mark as installed
        const newInstalled: InstalledVersion = {
          id: version.id,
          type: version.type,
          installed: true,
        };
        const updated = installedVersions.filter(v => v.id !== version.id);
        updated.unshift(newInstalled);
        saveInstalledVersions(updated);
      } else {
        setDownloadError(result.error || 'Download failed');
      }
    } catch {
      setDownloadError('Download failed. Please try again.');
    } finally {
      setDownloading(null);
      setDownloadProgress(prev => {
        const next = { ...prev };
        delete next[version.id];
        return next;
      });
    }
  };

  const handleDelete = (versionId: string) => {
    const updated = installedVersions.filter(v => v.id !== versionId);
    saveInstalledVersions(updated);
  };

  const isInstalled = (versionId: string) => {
    return installedVersions.some(v => v.id === versionId && v.installed);
  };

  // Version-type chip colors come from the shared semantic palette.
  const getVersionTypeColor = (type: string) => {
    switch (type) {
      case 'release':
        return tokens.color.success;
      case 'snapshot':
        return tokens.color.warning;
      case 'old_beta':
        return tokens.color.info;
      default:
        return 'rgba(255, 255, 255, 0.24)';
    }
  };

  const formatDate = (dateString?: string) => {
    if (!dateString) return '';
    const date = new Date(dateString);
    return date.toLocaleDateString();
  };

  // Install progress row: readable step name + determinate bar (launcher
  // research 03 §2.3: current step + percentage, not a bare spinner).
  const renderProgress = (versionId: string) => {
    const progress = downloadProgress[versionId];
    const stepLabel = t(installStepKey(progress?.stage || ''));
    const pct = progress?.pct ?? null;
    return (
      <Box sx={{ width: 180 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 0.5 }}>
          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
            {stepLabel}
          </Typography>
          {pct !== null && (
            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
              {pct}%
            </Typography>
          )}
        </Box>
        <LinearProgress
          variant={pct === null ? 'indeterminate' : 'determinate'}
          value={pct ?? 0}
          sx={{
            height: 6,
            borderRadius: 3,
            backgroundColor: 'rgba(255, 255, 255, 0.12)',
            '& .MuiLinearProgress-bar': { backgroundColor: tokens.color.accent, borderRadius: 3 },
          }}
        />
      </Box>
    );
  };

  const versionRowActions = (version: MCVersion) => (
    <ListItemSecondaryAction>
      {downloading === version.id ? (
        renderProgress(version.id)
      ) : isInstalled(version.id) ? (
        <Chip
          size="small"
          label={t('versions.installedChip')}
          sx={{ height: 24, backgroundColor: tokens.color.success, color: tokens.color.deep }}
        />
      ) : (
        <IconButton edge="end" onClick={() => handleDownload(version)} sx={{ color: tokens.color.accent }}>
          <Download />
        </IconButton>
      )}
    </ListItemSecondaryAction>
  );

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Button variant="outlined" startIcon={<Refresh />} onClick={fetchVersions} disabled={loading}>
          {t('versions.refresh')}
        </Button>
      </Box>

      <Tabs
        value={tabValue}
        onChange={(_, newValue) => setTabValue(newValue)}
        sx={{
          mb: 2,
          '& .MuiTab-root': { color: 'text.secondary' },
          '& .Mui-selected': { color: tokens.color.accent },
          '& .MuiTabs-indicator': { backgroundColor: tokens.color.accent },
        }}
      >
        <Tab label={t('versions.tab.installed')} />
        <Tab label={t('versions.tab.all')} />
        <Tab label={t('versions.tab.revolution')} />
      </Tabs>

      {tabValue === 1 && (
        <Box sx={{ mb: 2, display: 'flex', justifyContent: 'flex-end' }}>
          <FormControl sx={{ minWidth: 180 }}>
            <InputLabel sx={{ color: 'text.secondary' }}>{t('versions.filterType')}</InputLabel>
            <Select
              value={typeFilter}
              onChange={(e) => setTypeFilter(e.target.value)}
              sx={{
                color: 'text.primary',
                backgroundColor: 'background.paper',
                borderRadius: tokens.radius.md,
                '&:before': { borderColor: tokens.color.divider },
                '&:after': { borderColor: tokens.color.accent },
              }}
              label={t('versions.filterType')}
            >
              <MenuItem value="all">All</MenuItem>
              <MenuItem value="release">Release</MenuItem>
              <MenuItem value="snapshot">Snapshot</MenuItem>
              <MenuItem value="old_beta">Old Beta</MenuItem>
              <MenuItem value="old_alpha">Old Alpha</MenuItem>
            </Select>
          </FormControl>
        </Box>
      )}

      {downloadError && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setDownloadError('')}>
          {downloadError}
        </Alert>
      )}

      {loading ? (
        <LoadingSkeleton rows={6} />
      ) : fetchError ? (
        <ErrorState data={t('versions.listData')} onRetry={fetchVersions} />
      ) : tabValue === 0 ? (
        // Installed versions
        installedVersions.length === 0 ? (
          <EmptyState
            icon={<CloudDownload sx={{ fontSize: 64, mb: 2, opacity: 0.5, color: 'text.secondary' }} />}
            title={t('versions.emptyTitle')}
            hint={t('versions.emptyHint')}
            actionLabel={t('versions.browse')}
            onAction={() => setTabValue(1)}
          />
        ) : (
          <List sx={{ backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
            {installedVersions.map((version) => (
              <ListItem
                key={version.id}
                sx={{
                  cursor: 'pointer',
                  '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.06)' },
                }}
                onClick={() => onSelectVersion(version)}
              >
                <ListItemText
                  primary={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Typography sx={{ color: 'text.primary' }}>{version.id}</Typography>
                      <CheckCircle sx={{ fontSize: 16, color: tokens.color.success }} />
                    </Box>
                  }
                  secondary={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
                      <Chip
                        size="small"
                        label={version.type}
                        sx={{
                          height: 20,
                          fontSize: '0.7rem',
                          backgroundColor: getVersionTypeColor(version.type),
                          color: tokens.color.deep,
                        }}
                      />
                      {version.loaderType && (
                        <Chip
                          size="small"
                          label={`${version.loaderType} ${version.loaderVersion || ''}`}
                          sx={{
                            height: 20,
                            fontSize: '0.7rem',
                            backgroundColor: 'rgba(255, 255, 255, 0.12)',
                            color: 'text.secondary',
                          }}
                        />
                      )}
                    </Box>
                  }
                />
                <ListItemSecondaryAction>
                  <IconButton
                    edge="end"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDelete(version.id);
                    }}
                    sx={{ color: tokens.color.danger }}
                  >
                    <Delete />
                  </IconButton>
                </ListItemSecondaryAction>
              </ListItem>
            ))}
          </List>
        )
      ) : tabValue === 1 ? (
        // All versions
        <List sx={{ backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
          {versions
            .filter((version) => typeFilter === 'all' || version.type === typeFilter)
            .map((version) => (
            <ListItem
              key={version.id}
              sx={{ '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.06)' } }}
            >
              <ListItemText
                primary={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography sx={{ color: 'text.primary' }}>{version.id}</Typography>
                    {isInstalled(version.id) && (
                      <CheckCircle sx={{ fontSize: 16, color: tokens.color.success }} />
                    )}
                  </Box>
                }
                secondary={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
                    <Chip
                      size="small"
                      label={version.type}
                      sx={{
                        height: 20,
                        fontSize: '0.7rem',
                        backgroundColor: getVersionTypeColor(version.type),
                        color: tokens.color.deep,
                      }}
                    />
                    {version.releaseTime && (
                      <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {formatDate(version.releaseTime)}
                      </Typography>
                    )}
                  </Box>
                }
              />
              {versionRowActions(version)}
            </ListItem>
          ))}
        </List>
      ) : (
        // Revolution versions
        <Box>
          <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
            There are some versions that are highly recommended if you are a Revolution player.
          </Typography>
          <List sx={{ backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
            {REVOLUTION_VERSIONS.map((id) => {
              const version = versions.find((v) => v.id === id) || { id, type: 'release' };
              return (
                <ListItem
                  key={id}
                  sx={{ '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.06)' } }}
                >
                  <ListItemText
                    primary={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography sx={{ color: 'text.primary' }}>{id}</Typography>
                        {isInstalled(id) && (
                          <CheckCircle sx={{ fontSize: 16, color: tokens.color.success }} />
                        )}
                      </Box>
                    }
                    secondary={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
                        <Chip
                          size="small"
                          label={version.type}
                          sx={{
                            height: 20,
                            fontSize: '0.7rem',
                            backgroundColor: getVersionTypeColor(version.type),
                            color: tokens.color.deep,
                          }}
                        />
                        {version.releaseTime && (
                          <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                            {formatDate(version.releaseTime)}
                          </Typography>
                        )}
                      </Box>
                    }
                  />
                  {versionRowActions(version)}
                </ListItem>
              );
            })}
          </List>
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
