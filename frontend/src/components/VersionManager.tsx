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
  Paper,
  Tabs,
  Tab,
  CircularProgress,
  Alert,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
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

export default function VersionManager({ onSelectVersion }: VersionManagerProps) {
  const [versions, setVersions] = useState<MCVersion[]>([]);
  const [installedVersions, setInstalledVersions] = useState<InstalledVersion[]>([]);
  const [loading, setLoading] = useState(false);
  const [downloading, setDownloading] = useState<string | null>(null);
  const [downloadProgress, setDownloadProgress] = useState<Record<string, string>>({});
  const [error, setError] = useState('');
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
      let label = p.stage || '';
      if (p.total && p.total > 0 && typeof p.downloaded === 'number') {
        const pct = Math.min(100, Math.floor((p.downloaded / p.total) * 100));
        label = `${p.stage || ''} ${pct}%`;
      }
      setDownloadProgress(prev => ({ ...prev, [p.version_id as string]: label }));
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
    setError('');
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
          setError('Invalid version data format');
        }
      } else {
        setError(result.error || 'Failed to fetch versions');
      }
    } catch {
      setError('Failed to fetch versions. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  const handleDownload = async (version: MCVersion) => {
    setDownloading(version.id);
    setError('');
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
        setError(result.error || 'Download failed');
      }
    } catch {
      setError('Download failed. Please try again.');
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

  const getVersionTypeColor = (type: string) => {
    switch (type) {
      case 'release':
        return '#4CAF50';
      case 'snapshot':
        return '#FF9800';
      case 'old_beta':
        return '#9C27B0';
      case 'old_alpha':
        return '#795548';
      default:
        return '#9CA3af';
    }
  };

  const formatDate = (dateString?: string) => {
    if (!dateString) return '';
    const date = new Date(dateString);
    return date.toLocaleDateString();
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Button
          variant="outlined"
          startIcon={<Refresh />}
          onClick={fetchVersions}
          disabled={loading}
          sx={{ borderColor: '#4CAF50', color: '#4CAF50' }}
        >
          Refresh
        </Button>
      </Box>

      <Tabs
        value={tabValue}
        onChange={(_, newValue) => setTabValue(newValue)}
        sx={{
          mb: 2,
          '& .MuiTab-root': { color: '#9ca3af' },
          '& .Mui-selected': { color: '#4CAF50' },
          '& .MuiTabs-indicator': { backgroundColor: '#4CAF50' },
        }}
      >
        <Tab label="Installed" />
        <Tab label="All Versions" />
        <Tab label="Revolution" />
      </Tabs>

      {tabValue === 1 && (
        <Box sx={{ mb: 2, display: 'flex', justifyContent: 'flex-end' }}>
          <FormControl sx={{ minWidth: 180 }}>
            <InputLabel sx={{ color: '#9ca3af' }}>Filter by Type</InputLabel>
            <Select
              value={typeFilter}
              onChange={(e) => setTypeFilter(e.target.value)}
              sx={{
                color: 'white',
                backgroundColor: '#2d3339',
                '&:before': { borderColor: '#4CAF50' },
                '&:after': { borderColor: '#4CAF50' },
              }}
              label="Filter by Type"
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

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>
          {error}
        </Alert>
      )}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
          <CircularProgress sx={{ color: '#4CAF50' }} />
        </Box>
      ) : tabValue === 0 ? (
        // Installed versions
        installedVersions.length === 0 ? (
          <Paper
            sx={{
              p: 4,
              textAlign: 'center',
              backgroundColor: '#2d3339',
              color: '#9ca3af',
            }}
          >
            <CloudDownload sx={{ fontSize: 64, mb: 2, opacity: 0.5 }} />
            <Typography variant="h6" sx={{ mb: 1 }}>
              No Versions Installed
            </Typography>
            <Typography variant="body2" sx={{ mb: 2 }}>
              Go to "All Versions" tab to download Minecraft versions
            </Typography>
          </Paper>
        ) : (
          <List sx={{ backgroundColor: '#282c34', borderRadius: 1 }}>
            {installedVersions.map((version) => (
              <ListItem
                key={version.id}
                sx={{
                  cursor: 'pointer',
                  '&:hover': { backgroundColor: '#323842' },
                }}
                onClick={() => onSelectVersion(version)}
              >
                <ListItemText
                  primary={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Typography sx={{ color: 'white' }}>{version.id}</Typography>
                      <CheckCircle sx={{ fontSize: 16, color: '#4CAF50' }} />
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
                          color: 'white',
                        }}
                      />
                      {version.loaderType && (
                        <Chip
                          size="small"
                          label={`${version.loaderType} ${version.loaderVersion || ''}`}
                          sx={{
                            height: 20,
                            fontSize: '0.7rem',
                            backgroundColor: '#374151',
                            color: '#9ca3af',
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
                    sx={{ color: '#ef4444' }}
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
        <List sx={{ backgroundColor: '#282c34', borderRadius: 1 }}>
          {versions
            .filter((version) => typeFilter === 'all' || version.type === typeFilter)
            .map((version) => (
            <ListItem
              key={version.id}
              sx={{
                '&:hover': { backgroundColor: '#323842' },
              }}
            >
              <ListItemText
                primary={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography sx={{ color: 'white' }}>{version.id}</Typography>
                    {isInstalled(version.id) && (
                      <CheckCircle sx={{ fontSize: 16, color: '#4CAF50' }} />
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
                        color: 'white',
                      }}
                    />
                    {version.releaseTime && (
                      <Typography variant="caption" sx={{ color: '#6b7280' }}>
                        {formatDate(version.releaseTime)}
                      </Typography>
                    )}
                  </Box>
                }
              />
              <ListItemSecondaryAction>
                {downloading === version.id ? (
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    {downloadProgress[version.id] && (
                      <Typography variant="caption" sx={{ color: '#9ca3af', fontSize: '0.7rem' }}>
                        {downloadProgress[version.id]}
                      </Typography>
                    )}
                    <CircularProgress size={24} sx={{ color: '#4CAF50' }} />
                  </Box>
                ) : isInstalled(version.id) ? (
                  <Chip
                    size="small"
                    label="Installed"
                    sx={{
                      height: 24,
                      backgroundColor: '#4CAF50',
                      color: 'white',
                    }}
                  />
                ) : (
                  <IconButton
                    edge="end"
                    onClick={() => handleDownload(version)}
                    sx={{ color: '#4CAF50' }}
                  >
                    <Download />
                  </IconButton>
                )}
              </ListItemSecondaryAction>
            </ListItem>
          ))}
        </List>
      ) : (
        // Revolution versions
        <Box>
          <Typography variant="body2" sx={{ color: '#9ca3af', mb: 2 }}>
            There are some versions that are highly recommended if you are a Revolution player.
          </Typography>
          <List sx={{ backgroundColor: '#282c34', borderRadius: 1 }}>
            {REVOLUTION_VERSIONS.map((id) => {
              const version = versions.find((v) => v.id === id) || { id, type: 'release' };
              return (
                <ListItem
                  key={id}
                  sx={{
                    '&:hover': { backgroundColor: '#323842' },
                  }}
                >
                  <ListItemText
                    primary={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Typography sx={{ color: 'white' }}>{id}</Typography>
                        {isInstalled(id) && (
                          <CheckCircle sx={{ fontSize: 16, color: '#4CAF50' }} />
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
                            color: 'white',
                          }}
                        />
                        {version.releaseTime && (
                          <Typography variant="caption" sx={{ color: '#6b7280' }}>
                            {formatDate(version.releaseTime)}
                          </Typography>
                        )}
                      </Box>
                    }
                  />
                  <ListItemSecondaryAction>
                    {downloading === id ? (
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        {downloadProgress[id] && (
                          <Typography variant="caption" sx={{ color: '#9ca3af', fontSize: '0.7rem' }}>
                            {downloadProgress[id]}
                          </Typography>
                        )}
                        <CircularProgress size={24} sx={{ color: '#4CAF50' }} />
                      </Box>
                    ) : isInstalled(id) ? (
                      <Chip
                        size="small"
                        label="Installed"
                        sx={{
                          height: 24,
                          backgroundColor: '#4CAF50',
                          color: 'white',
                        }}
                      />
                    ) : (
                      <IconButton
                        edge="end"
                        onClick={() => handleDownload(version)}
                        sx={{ color: '#4CAF50' }}
                      >
                        <Download />
                      </IconButton>
                    )}
                  </ListItemSecondaryAction>
                </ListItem>
              );
            })}
          </List>
        </Box>
      )}
    </Box>
  );
}
