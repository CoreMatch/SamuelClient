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
} from '@mui/icons-material';
import { StoredAccount } from '../accountStorage';
import { InstalledVersion } from '../types';
import { tokens } from '../theme';
import {
  GetJavaInstallations,
  GetInstalledVersions,
  GetFabricVersions,
  GetForgeVersions,
  InstallFabricLoader,
  InstallForgeLoader,
  LaunchGame,
} from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

interface ModLoader {
  name: string;
  version: string;
}

// Shapes of the backend payloads consumed below (see bindings.go).
interface BackendInstalledVersion {
  id: string;
  type: string;
  has_json: boolean;
  has_jar: boolean;
}

interface FabricLoaderEntry {
  loader: { version: string };
}

interface ForgeVersionEntry {
  version: string;
}

interface JavaInstallation {
  id: number;
  path: string;
  friendly_name: string;
  version: string;
  is_default: boolean;
}

// LiveLogLine is one line of game output streamed over the "launcher:log"
// Wails event (ADR-0004).
interface LiveLogLine {
  source: string;
  line: string;
  ts: string;
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
  const [liveLogs, setLiveLogs] = useState<LiveLogLine[]>([]);
  const [showLogs, setShowLogs] = useState(false);
  const [javaInstallations, setJavaInstallations] = useState<JavaInstallation[]>([]);
  const [selectedJava, setSelectedJava] = useState<JavaInstallation | null>(null);
  const [installingLoader, setInstallingLoader] = useState<string | null>(null);

  useEffect(() => {
    loadVersions();
    loadSettings();
    loadJavaInstallations();
  }, []);

  // ADR-0004: live launch logs are pushed by the backend as "launcher:log"
  // events ({source: "stdout"|"stderr", line: string}); subscribe once and
  // unsubscribe on unmount. No HTTP polling involved.
  useEffect(() => {
    EventsOn('launcher:log', (payload: { source?: string; line?: string }) => {
      if (!payload) return;
      setLiveLogs(prev => [
        ...prev,
        {
          source: payload.source || 'stdout',
          line: payload.line || '',
          ts: new Date().toLocaleTimeString(),
        },
      ]);
    });
    return () => {
      EventsOff('launcher:log');
    };
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
  }, [currentVersion, onVersionSelect]);

  const loadVersions = async () => {
    try {
      // Prefer localStorage (lets the user mark versions as installed for modded setups),
      // then fall back to a disk scan from the backend for any version that has a JSON+jar.
      const stored = localStorage.getItem('installed_versions');
      const local: InstalledVersion[] = stored ? JSON.parse(stored) : [];

      try {
        const result = await GetInstalledVersions();
        if (result.success && Array.isArray(result.data?.versions)) {
          const backendVersions = result.data.versions as BackendInstalledVersion[];
          const fromDisk: InstalledVersion[] = backendVersions
            .filter(v => v.has_json && v.has_jar)
            .map(v => ({
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
        (fabricResult.data as FabricLoaderEntry[]).slice(0, 5).forEach(f => {
          loaderList.push({
            name: `fabric-${f.loader.version}`,
            version: f.loader.version,
          });
        });
      }

      if (forgeResult.success && Array.isArray(forgeResult.data)) {
        (forgeResult.data as ForgeVersionEntry[]).slice(0, 5).forEach(f => {
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

  const handleVersionChange = (versionId: string) => {
    const version = versions.find(v => v.id === versionId);
    setCurrentVersion(version || null);
    if (version) {
      fetchLoaders(version.id);
    }
  };

  const handleLoaderChange = async (loaderName: string) => {
    const loader = loaders.find(l => l.name === loaderName);
    saveSettings({
      ...settings,
      selectedLoader: loaderName,
      loaderVersion: loader?.version || '',
    });

    if (!loader || loader.name === 'none' || !loader.version) {
      return;
    }
    if (!currentVersion) {
      setError('Please select a Minecraft version first');
      return;
    }

    // Loader versions install through the dedicated bindings: Fabric writes a
    // profile JSON inheriting the vanilla version, Forge runs its installer.
    // The resulting version shows up in the version list afterwards.
    setError('');
    setInstallingLoader(loaderName);
    try {
      const result = loader.name.startsWith('fabric-')
        ? await InstallFabricLoader(currentVersion.id, loader.version, '')
        : await InstallForgeLoader(currentVersion.id, loader.version, '');
      if (result.success) {
        await loadVersions();
      } else {
        setError(result.error || 'Failed to install loader');
      }
    } catch {
      setError('Failed to install loader. Please try again.');
    } finally {
      setInstallingLoader(null);
    }
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
    setLiveLogs([]);

    try {
      // ADR-0004: the launch request (including access/client tokens) goes
      // through the in-process Wails binding, not a local HTTP socket. Live
      // game output arrives separately over "launcher:log" events.
      const result = await LaunchGame({
        version_id: currentVersion.id,
        version_type: currentVersion.type,
        username: currentAccount.username,
        uuid: currentAccount.uuid,
        access_token: currentAccount.accessToken,
        client_token: currentAccount.clientToken,
        auth_type: currentAccount.authType,
        auth_server: currentAccount.authServer || '',
        authlib_jar: localStorage.getItem('authlib_jar_path') || '',
        authlib_meta: '',
        game_dir: '',
        java_path: '',
        loader_type: settings.selectedLoader === 'none' ? '' : settings.selectedLoader.split('-')[0],
        loader_version: settings.loaderVersion,
        join_server: '',
        join_port: 0,
        max_memory: settings.maxMemory,
        min_memory: settings.minMemory,
        java_args: settings.javaArgs,
        game_width: settings.gameWidth,
        game_height: settings.gameHeight,
      });

      if (result.success) {
        console.log('Game launched with PID:', result.data?.pid);
      } else {
        setError(result.error || 'Failed to launch game');
      }
    } catch {
      setError('Failed to launch game. Please try again.');
    } finally {
      setLaunching(false);
    }
  };

  const memoryMarks = [
    { value: 512, label: '512MB' },
    { value: 1024, label: '1GB' },
    { value: 2048, label: '2GB' },
    { value: 4096, label: '4GB' },
    { value: 8192, label: '8GB' },
  ];

  return (
    <Box>

      <Collapse in={!!error}>
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>
          {error}
        </Alert>
      </Collapse>

      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' }, gap: 3 }}>
        {/* Left Panel - Version Selection */}
        <Paper sx={{ p: 3, backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
          <Typography variant="h6" sx={{ color: 'white', mb: 2 }}>
            Select Version
          </Typography>

          <FormControl fullWidth sx={{ mb: 3 }}>
            <InputLabel sx={{ color: 'text.secondary' }}>Minecraft Version</InputLabel>
            <Select
              value={currentVersion?.id || ''}
              onChange={(e) => handleVersionChange(e.target.value)}
              label="Minecraft Version"
              sx={{
                color: 'white',
                '.MuiOutlinedInput-notchedOutline': { borderColor: tokens.color.divider },
                '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: tokens.color.accent },
                '.MuiSvgIcon-root': { color: 'text.secondary' },
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
            <InputLabel sx={{ color: 'text.secondary' }}>Mod Loader</InputLabel>
            <Select
              value={settings.selectedLoader}
              onChange={(e) => handleLoaderChange(e.target.value)}
              label="Mod Loader"
              disabled={installingLoader !== null}
              sx={{
                color: 'white',
                '.MuiOutlinedInput-notchedOutline': { borderColor: tokens.color.divider },
                '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: tokens.color.accent },
                '.MuiSvgIcon-root': { color: 'text.secondary' },
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
            <InputLabel sx={{ color: 'text.secondary' }}>Java</InputLabel>
            <Select
              value={selectedJava?.id || ''}
              onChange={(e) => {
                const java = javaInstallations.find(j => j.id === e.target.value);
                setSelectedJava(java || null);
              }}
              label="Java"
              sx={{
                color: 'white',
                '.MuiOutlinedInput-notchedOutline': { borderColor: tokens.color.divider },
                '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: tokens.color.accent },
                '.MuiSvgIcon-root': { color: 'text.secondary' },
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

          <Divider sx={{ borderColor: tokens.color.divider, my: 2 }} />

          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
            <Typography variant="subtitle2" sx={{ color: 'text.secondary' }}>
              Account
            </Typography>
          </Box>

          {currentAccount ? (
            <Box sx={{ backgroundColor: 'rgba(255, 255, 255, 0.06)', p: 2, borderRadius: 1 }}>
              <Typography sx={{ color: 'white' }}>{currentAccount.username}</Typography>
              <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                {currentAccount.authType === 'authlib-injector' ? 'Online' : 'Offline'}
              </Typography>
            </Box>
          ) : (
            <Typography sx={{ color: tokens.color.danger }}>No account selected</Typography>
          )}
        </Paper>

        {/* Right Panel - Settings */}
        <Paper sx={{ p: 3, backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
          <Box
            sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2, cursor: 'pointer' }}
            onClick={() => setShowSettings(!showSettings)}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Settings sx={{ color: 'text.secondary' }} />
              <Typography variant="h6" sx={{ color: 'white' }}>
                Launch Settings
              </Typography>
            </Box>
            {showSettings ? <ExpandLess sx={{ color: 'text.secondary' }} /> : <ExpandMore sx={{ color: 'text.secondary' }} />}
          </Box>

          <Collapse in={!showSettings}>
            <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
              Memory: {settings.maxMemory}MB | Resolution: {settings.gameWidth}x{settings.gameHeight}
            </Typography>
          </Collapse>

          <Collapse in={showSettings}>
            <Box sx={{ pt: 1 }}>
              <Typography gutterBottom sx={{ color: 'text.secondary', fontSize: '0.875rem' }}>
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
                  color: tokens.color.accent,
                  '& .MuiSlider-markLabel': { color: 'text.secondary', fontSize: '0.7rem' },
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
                  '& .MuiInputLabel-root': { color: 'text.secondary' },
                  '& .MuiOutlinedInput-notchedOutline': { borderColor: tokens.color.divider },
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
                    '& .MuiInputLabel-root': { color: 'text.secondary' },
                    '& .MuiOutlinedInput-notchedOutline': { borderColor: tokens.color.divider },
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
                    '& .MuiInputLabel-root': { color: 'text.secondary' },
                    '& .MuiOutlinedInput-notchedOutline': { borderColor: tokens.color.divider },
                  }}
                />
              </Box>
            </Box>
          </Collapse>
        </Paper>
      </Box>

      {/* Logs Panel */}
      <Collapse in={showLogs}>
        <Paper sx={{ p: 3, backgroundColor: 'background.paper', borderRadius: tokens.radius.lg, mt: 3 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
            <Typography variant="h6" sx={{ color: 'white' }}>
              Logs
            </Typography>
          </Box>

          <Box sx={{ height: 200, backgroundColor: tokens.color.deep, borderRadius: 1, overflowY: 'auto', p: 2 }}>
              {liveLogs.length > 0 ? (
                // ADR-0004: live game output streamed over "launcher:log".
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                  {liveLogs.map((l, i) => (
                    <Box key={i} sx={{ display: 'flex', gap: 2, fontSize: '0.875rem' }}>
                      <span style={{ color: tokens.color.textMuted, fontSize: '0.75rem' }}>{l.ts}</span>
                      <span style={{ color: l.source === 'stderr' ? tokens.color.warning : tokens.color.info, fontWeight: 'bold' }}>
                        [{l.source.toUpperCase()}]
                      </span>
                      <span style={{ color: tokens.color.text, whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>{l.line}</span>
                    </Box>
                  ))}
                </Box>
              ) : (
                <Typography sx={{ color: 'text.secondary', textAlign: 'center', py: 4 }}>
                  No logs available
                </Typography>
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
          sx={{ borderColor: tokens.color.divider, color: 'text.secondary' }}
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
            backgroundColor: tokens.color.accent,
            fontSize: '1.1rem',
            '&:hover': { backgroundColor: tokens.color.accentDark },
            '&:disabled': { backgroundColor: 'rgba(255, 255, 255, 0.12)', color: 'text.secondary' },
          }}
        >
          {launching ? 'Launching...' : 'Play'}
        </Button>
      </Box>
    </Box>
  );
}
