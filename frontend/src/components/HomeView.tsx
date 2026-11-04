import { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Button,
  Paper,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Slider,
  TextField,
  CircularProgress,
  Alert,
  Collapse,
  Divider,
} from '@mui/material';
import {
  PlayArrow,
  Settings,
  ExpandMore,
  ExpandLess,
  Refresh,
} from '@mui/icons-material';
import { StoredAccount } from './AccountManager';
import {
  GetJavaInstallations,
  GetInstalledVersions,
  GetFabricVersions,
  GetForgeVersions,
  GetLogs,
} from '../../wailsjs/go/main/App';

interface InstalledVersion {
  id: string;
  type: string;
  installed: boolean;
  loaderType?: string;
  loaderVersion?: string;
}

interface ModLoader {
  name: string;
  version: string;
}

interface JavaInstallation {
  id: number;
  path: string;
  friendly_name: string;
  version: string;
  is_default: boolean;
}

interface LogEntry {
  id: number;
  level: string;
  message: string;
  source?: string;
  created_at: string;
}

interface HomeViewProps {
  selectedVersion: InstalledVersion | null;
  onVersionSelect: (version: InstalledVersion | null) => void;
  currentAccount: StoredAccount | null;
  onOpenSettings: () => void;
}

const STORAGE_KEY = 'launcher_settings';

interface LauncherSettings {
  maxMemory: number;
  minMemory: number;
  javaArgs: string;
  gameWidth: number;
  gameHeight: number;
  selectedLoader: string;
  loaderVersion: string;
}

const DEFAULT_SETTINGS: LauncherSettings = {
  maxMemory: 4096,
  minMemory: 1024,
  javaArgs: '',
  gameWidth: 854,
  gameHeight: 480,
  selectedLoader: 'none',
  loaderVersion: '',
};

export default function HomeView({
  selectedVersion,
  onVersionSelect,
  currentAccount,
  onOpenSettings,
}: HomeViewProps) {
  const [versions, setVersions] = useState<InstalledVersion[]>([]);
  const [currentVersion, setCurrentVersion] = useState<InstalledVersion | null>(selectedVersion);
  const [loaders, setLoaders] = useState<ModLoader[]>([]);
  const [settings, setSettings] = useState<LauncherSettings>(DEFAULT_SETTINGS);
  const [showSettings, setShowSettings] = useState(false);
  const [launching, setLaunching] = useState(false);
  const [error, setError] = useState('');
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [showLogs, setShowLogs] = useState(false);
  const [logsLoading, setLogsLoading] = useState(false);
  const [javaInstallations, setJavaInstallations] = useState<JavaInstallation[]>([]);
  const [selectedJava, setSelectedJava] = useState<JavaInstallation | null>(null);

  useEffect(() => {
    loadVersions();
    loadSettings();
    loadJavaInstallations();
  }, []);

  const loadJavaInstallations = async () => {
    try {
      const result = await GetJavaInstallations();
      if (result.success && Array.isArray(result.data)) {
        const data = result.data as JavaInstallation[];
        setJavaInstallations(data);
        const defaultJava = data.find((j: JavaInstallation) => j.is_default);
        if (defaultJava) {
          setSelectedJava(defaultJava);
        } else if (data.length > 0) {
          setSelectedJava(data[0]);
        }
      }
    } catch (err) {
      console.error('Failed to load Java installations:', err);
    }
  };

  useEffect(() => {
    if (currentVersion) {
      onVersionSelect(currentVersion);
    }
  }, [currentVersion]);

  const loadVersions = async () => {
    try {
      // Prefer localStorage (lets the user mark versions as installed for modded setups),
      // then fall back to a disk scan from the backend for any version that has a JSON+jar.
      const stored = localStorage.getItem('installed_versions');
      const local: InstalledVersion[] = stored ? JSON.parse(stored) : [];

      try {
        const result = await GetInstalledVersions();
        if (result.success && Array.isArray(result.data?.versions)) {
          const fromDisk: InstalledVersion[] = result.data.versions
            .filter((v: any) => v.has_json && v.has_jar)
            .map((v: any) => ({
              id: v.id,
              type: v.type || 'release',
              installed: true,
            }));
          // merge, keep localStorage order, dedupe by id
          const seen = new Set(local.map(v => v.id));
          const merged = [...local, ...fromDisk.filter(v => !seen.has(v.id))];
          setVersions(merged);
          return;
        }
      } catch (e) {
        console.error('Failed to query installed versions from backend:', e);
      }
      setVersions(local);
    } catch (err) {
      console.error('Failed to load versions:', err);
    }
  };

  const loadSettings = () => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        setSettings({ ...DEFAULT_SETTINGS, ...JSON.parse(stored) });
      }
    } catch (err) {
      console.error('Failed to load settings:', err);
    }
  };

  const saveSettings = (newSettings: LauncherSettings) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(newSettings));
      setSettings(newSettings);
    } catch (err) {
      console.error('Failed to save settings:', err);
    }
  };

  const fetchLoaders = async (mcVersion: string) => {
    try {
      const [fabricResult, forgeResult] = await Promise.all([
        GetFabricVersions(mcVersion),
        GetForgeVersions(mcVersion),
      ]);

      const loaderList: ModLoader[] = [{ name: 'none', version: '' }];

      if (fabricResult.success && Array.isArray(fabricResult.data)) {
        fabricResult.data.slice(0, 5).forEach((f: any) => {
          loaderList.push({
            name: `fabric-${f.loader.version}`,
            version: f.loader.version,
          });
        });
      }

      if (forgeResult.success && Array.isArray(forgeResult.data)) {
        forgeResult.data.slice(0, 5).forEach((f: any) => {
          loaderList.push({
            name: `forge-${f.version}`,
            version: f.version,
          });
        });
      }

      setLoaders(loaderList);
    } catch (err) {
      console.error('Failed to fetch loaders:', err);
    }
  };

  const fetchLogs = async () => {
    setLogsLoading(true);
    try {
      const result = await GetLogs(100);
      if (result.success && Array.isArray(result.data)) {
        setLogs(result.data);
      }
    } catch (err) {
      console.error('Failed to fetch logs:', err);
    } finally {
      setLogsLoading(false);
    }
  };

  const handleVersionChange = (versionId: string) => {
    const version = versions.find(v => v.id === versionId);
    setCurrentVersion(version || null);
    if (version) {
      fetchLoaders(version.id);
    }
  };

  const handleLoaderChange = (loaderName: string) => {
    const loader = loaders.find(l => l.name === loaderName);
    saveSettings({
      ...settings,
      selectedLoader: loaderName,
      loaderVersion: loader?.version || '',
    });
  };

  const handleSettingChange = (key: keyof LauncherSettings, value: number | string) => {
    saveSettings({
      ...settings,
      [key]: value,
    });
  };

  const handleLaunch = async () => {
    if (!currentVersion || !currentAccount) {
      setError('Please select a version and account');
      return;
    }

    setLaunching(true);
    setError('');
    setShowLogs(true);

    try {
      const response = await fetch('http://localhost:34501/api/v1/launcher/launch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          version_id: currentVersion.id,
          version_type: currentVersion.type,
          username: currentAccount.username,
          uuid: currentAccount.uuid,
          access_token: currentAccount.accessToken,
          client_token: currentAccount.clientToken,
          auth_type: currentAccount.authType,
          auth_server: currentAccount.authServer || '',
          authlib_jar: localStorage.getItem('authlib_jar_path') || '',
          loader_type: settings.selectedLoader === 'none' ? '' : settings.selectedLoader.split('-')[0],
          loader_version: settings.loaderVersion,
          max_memory: settings.maxMemory,
          min_memory: settings.minMemory,
          java_args: settings.javaArgs,
          game_width: settings.gameWidth,
          game_height: settings.gameHeight,
        }),
      });

      const data = await response.json();
      if (data.success) {
        console.log('Game launched with PID:', data.data?.pid);
      } else {
        setError(data.error || 'Failed to launch game');
      }
    } catch (err) {
      setError('Failed to launch game. Please try again.');
    } finally {
      setLaunching(false);
      fetchLogs();
    }
  };

  const memoryMarks = [
    { value: 512, label: '512MB' },
    { value: 1024, label: '1GB' },
    { value: 2048, label: '2GB' },
    { value: 4096, label: '4GB' },
    { value: 8192, label: '8GB' },
  ];

  const getLogLevelColor = (level: string) => {
    switch (level.toLowerCase()) {
      case 'error':
        return '#ef4444';
      case 'warn':
        return '#f59e0b';
      case 'info':
        return '#3b82f6';
      case 'debug':
        return '#8b5cf6';
      default:
        return '#9ca3af';
    }
  };

  return (
    <Box>

      <Collapse in={!!error}>
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>
          {error}
        </Alert>
      </Collapse>

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' }, gap: 3 }}>
        {/* Left Panel - Version Selection */}
        <Paper sx={{ p: 3, backgroundColor: '#282c34' }}>
          <Typography variant="h6" sx={{ color: 'white', mb: 2 }}>
            Select Version
          </Typography>

          <FormControl fullWidth sx={{ mb: 3 }}>
            <InputLabel sx={{ color: '#9ca3af' }}>Minecraft Version</InputLabel>
            <Select
              value={currentVersion?.id || ''}
              onChange={(e) => handleVersionChange(e.target.value)}
              label="Minecraft Version"
              sx={{
                color: 'white',
                '.MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
                '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#4CAF50' },
                '.MuiSvgIcon-root': { color: '#9ca3af' },
              }}
            >
              {versions.length === 0 ? (
                <MenuItem disabled value="">
                  No versions installed
                </MenuItem>
              ) : (
                versions.map((v) => (
                  <MenuItem key={v.id} value={v.id}>
                    {v.id}
                  </MenuItem>
                ))
              )}
            </Select>
          </FormControl>

          <FormControl fullWidth sx={{ mb: 3 }}>
            <InputLabel sx={{ color: '#9ca3af' }}>Mod Loader</InputLabel>
            <Select
              value={settings.selectedLoader}
              onChange={(e) => handleLoaderChange(e.target.value)}
              label="Mod Loader"
              sx={{
                color: 'white',
                '.MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
                '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#4CAF50' },
                '.MuiSvgIcon-root': { color: '#9ca3af' },
              }}
            >
              {loaders.map((l) => (
                <MenuItem key={l.name} value={l.name}>
                  {l.name === 'none' ? 'None' : l.name.replace('-', ' ')}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl fullWidth sx={{ mb: 3 }}>
            <InputLabel sx={{ color: '#9ca3af' }}>Java</InputLabel>
            <Select
              value={selectedJava?.id || ''}
              onChange={(e) => {
                const java = javaInstallations.find(j => j.id === e.target.value);
                setSelectedJava(java || null);
              }}
              label="Java"
              sx={{
                color: 'white',
                '.MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
                '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#4CAF50' },
                '.MuiSvgIcon-root': { color: '#9ca3af' },
              }}
            >
              {javaInstallations.length === 0 ? (
                <MenuItem disabled value="">
                  No Java installed
                </MenuItem>
              ) : (
                javaInstallations.map((j) => (
                  <MenuItem key={j.id} value={j.id}>
                    {j.friendly_name || `Java ${j.version}`} ({j.version})
                  </MenuItem>
                ))
              )}
            </Select>
          </FormControl>

          <Divider sx={{ borderColor: '#374151', my: 2 }} />

          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
            <Typography variant="subtitle2" sx={{ color: '#9ca3af' }}>
              Account
            </Typography>
          </Box>

          {currentAccount ? (
            <Box sx={{ backgroundColor: '#2d3339', p: 2, borderRadius: 1 }}>
              <Typography sx={{ color: 'white' }}>{currentAccount.username}</Typography>
              <Typography variant="caption" sx={{ color: '#6b7280' }}>
                {currentAccount.authType === 'authlib-injector' ? 'Online' : 'Offline'}
              </Typography>
            </Box>
          ) : (
            <Typography sx={{ color: '#ef4444' }}>No account selected</Typography>
          )}
        </Paper>

        {/* Right Panel - Settings */}
        <Paper sx={{ p: 3, backgroundColor: '#282c34' }}>
          <Box
            sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2, cursor: 'pointer' }}
            onClick={() => setShowSettings(!showSettings)}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Settings sx={{ color: '#9ca3af' }} />
              <Typography variant="h6" sx={{ color: 'white' }}>
                Launch Settings
              </Typography>
            </Box>
            {showSettings ? <ExpandLess sx={{ color: '#9ca3af' }} /> : <ExpandMore sx={{ color: '#9ca3af' }} />}
          </Box>

          <Collapse in={!showSettings}>
            <Typography variant="body2" sx={{ color: '#6b7280', mb: 2 }}>
              Memory: {settings.maxMemory}MB | Resolution: {settings.gameWidth}x{settings.gameHeight}
            </Typography>
          </Collapse>

          <Collapse in={showSettings}>
            <Box sx={{ pt: 1 }}>
              <Typography gutterBottom sx={{ color: '#9ca3af', fontSize: '0.875rem' }}>
                Maximum Memory: {settings.maxMemory}MB
              </Typography>
              <Slider
                value={settings.maxMemory}
                onChange={(_, value) => handleSettingChange('maxMemory', value as number)}
                min={512}
                max={16384}
                step={256}
                marks={memoryMarks}
                sx={{
                  color: '#4CAF50',
                  '& .MuiSlider-markLabel': { color: '#6b7280', fontSize: '0.7rem' },
                }}
              />

              <TextField
                fullWidth
                label="JVM Arguments"
                value={settings.javaArgs}
                onChange={(e) => handleSettingChange('javaArgs', e.target.value)}
                placeholder="-XX:+UseG1GC"
                size="small"
                sx={{
                  mt: 2,
                  '& .MuiInputBase-input': { color: 'white' },
                  '& .MuiInputLabel-root': { color: '#9ca3af' },
                  '& .MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
                }}
              />

              <Box sx={{ display: 'flex', gap: 2, mt: 2 }}>
                <TextField
                  label="Width"
                  type="number"
                  value={settings.gameWidth}
                  onChange={(e) => handleSettingChange('gameWidth', parseInt(e.target.value) || 854)}
                  size="small"
                  sx={{
                    flex: 1,
                    '& .MuiInputBase-input': { color: 'white' },
                    '& .MuiInputLabel-root': { color: '#9ca3af' },
                    '& .MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
                  }}
                />
                <TextField
                  label="Height"
                  type="number"
                  value={settings.gameHeight}
                  onChange={(e) => handleSettingChange('gameHeight', parseInt(e.target.value) || 480)}
                  size="small"
                  sx={{
                    flex: 1,
                    '& .MuiInputBase-input': { color: 'white' },
                    '& .MuiInputLabel-root': { color: '#9ca3af' },
                    '& .MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
                  }}
                />
              </Box>
            </Box>
          </Collapse>
        </Paper>
      </Box>

      {/* Logs Panel */}
      <Collapse in={showLogs}>
        <Paper sx={{ p: 3, backgroundColor: '#282c34', mt: 3 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
            <Typography variant="h6" sx={{ color: 'white' }}>
              Logs
            </Typography>
            <Button
              startIcon={<Refresh />}
              onClick={fetchLogs}
              disabled={logsLoading}
              sx={{ color: '#9ca3af', textTransform: 'none' }}
            >
              Refresh
            </Button>
          </Box>

          <Box sx={{ height: 200, backgroundColor: '#1f2937', borderRadius: 1, overflowY: 'auto', p: 2 }}>
              {logsLoading ? (
                <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                  <CircularProgress size={24} />
                </Box>
              ) : logs.length === 0 ? (
                <Typography sx={{ color: '#6b7280', textAlign: 'center', py: 4 }}>
                  No logs available
                </Typography>
              ) : (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                  {logs.map((log) => (
                    <Box key={log.id} sx={{ display: 'flex', gap: 2, fontSize: '0.875rem' }}>
                      <span style={{ color: '#6b7280', fontSize: '0.75rem' }}>
                        {new Date(log.created_at).toLocaleTimeString()}
                      </span>
                      <span style={{ color: getLogLevelColor(log.level), fontWeight: 'bold' }}>
                        [{log.level.toUpperCase()}]
                      </span>
                      <span style={{ color: '#e5e7eb' }}>
                        {log.source && <span style={{ color: '#9ca3af' }}>[{log.source}]</span>}
                        {log.message}
                      </span>
                    </Box>
                  ))}
                </Box>
              )}
          </Box>
        </Paper>
      </Collapse>

      {/* Launch Button */}
      <Box sx={{ mt: 3, display: 'flex', justifyContent: 'center', gap: 2 }}>
        <Button
          variant="outlined"
          startIcon={<Settings />}
          onClick={onOpenSettings}
          sx={{ borderColor: '#4a5568', color: '#9ca3af' }}
        >
          Settings
        </Button>
        <Button
          variant="contained"
          startIcon={launching ? <CircularProgress size={20} color="inherit" /> : <PlayArrow />}
          onClick={handleLaunch}
          disabled={launching || !currentVersion || !currentAccount}
          sx={{
            px: 4,
            py: 1.5,
            backgroundColor: '#4CAF50',
            fontSize: '1.1rem',
            '&:hover': { backgroundColor: '#45a049' },
            '&:disabled': { backgroundColor: '#374151', color: '#6b7280' },
          }}
        >
          {launching ? 'Launching...' : 'Play'}
        </Button>
      </Box>
    </Box>
  );
}
