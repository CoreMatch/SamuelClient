import { useState, useEffect } from 'react';
import { Box, Typography, Card, CardContent, Chip, Button, TextField, Dialog, DialogTitle, DialogContent, DialogActions, IconButton, Tooltip, Slider, Alert, Snackbar } from '@mui/material';
import { Add, Delete, Star, Edit, Check, X, Save, Memory } from '@mui/icons-material';
import {
  GetLauncherConfig,
  GetJavaInstallations,
  UpdateLauncherConfig,
  AddJavaInstallation,
  UpdateJavaInstallation,
  DeleteJavaInstallation,
  SetDefaultJavaInstallation,
} from '../../wailsjs/go/main/App';

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
    } catch (err) {
      setError('Failed to fetch Java installations');
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
      return `Minimum memory must be at least ${MIN_MEMORY_MB} MB`;
    }
    if (maxMemory < MIN_MEMORY_MB) {
      return `Maximum memory must be at least ${MIN_MEMORY_MB} MB`;
    }
    if (maxMemory > MAX_MEMORY_MB) {
      return `Maximum memory cannot exceed ${MAX_MEMORY_MB} MB`;
    }
    if (minMemory > maxMemory) {
      return 'Minimum memory cannot exceed maximum memory';
    }
    if (gameWidth < MIN_GAME_DIMENSION || gameWidth > MAX_GAME_DIMENSION) {
      return `Game width must be between ${MIN_GAME_DIMENSION} and ${MAX_GAME_DIMENSION}`;
    }
    if (gameHeight < MIN_GAME_DIMENSION || gameHeight > MAX_GAME_DIMENSION) {
      return `Game height must be between ${MIN_GAME_DIMENSION} and ${MAX_GAME_DIMENSION}`;
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
      } else {
        setSaveError(result.error || 'Failed to save settings');
      }
    } catch (err) {
      setSaveError('Failed to save settings');
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
      setDialogError('Java path is required');
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
        setDialogError(result.error || 'Failed to add Java installation');
      }
    } catch (err) {
      setDialogError('Failed to add Java installation');
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
        setDialogError(result.error || 'Failed to update Java installation');
      }
    } catch (err) {
      setDialogError('Failed to update Java installation');
    }
  };

  const handleDeleteJava = async (id: number) => {
    if (!confirm('Are you sure you want to delete this Java installation?')) return;

    try {
      const result = await DeleteJavaInstallation(id);
      if (result.success) {
        fetchJavaInstallations();
      } else {
        setError(result.error || 'Failed to delete Java installation');
      }
    } catch (err) {
      setError('Failed to delete Java installation');
    }
  };

  const handleSetDefault = async (id: number) => {
    try {
      const result = await SetDefaultJavaInstallation(id);
      if (result.success) {
        fetchJavaInstallations();
      } else {
        setError(result.error || 'Failed to set default Java');
      }
    } catch (err) {
      setError('Failed to set default Java');
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
    return (
      <Box>
        <Typography sx={{ color: '#9ca3af' }}>Loading...</Typography>
      </Box>
    );
  }

  return (
    <Box>
      {error && (
        <Typography sx={{ color: '#ef4444', marginBottom: 2 }}>{error}</Typography>
      )}

      <Card sx={{ backgroundColor: '#1f2937', marginBottom: 2 }}>
        <CardContent>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 2 }}>
            <Typography variant="h6" sx={{ color: 'white' }}>
              Java Installations
            </Typography>
            <Button
              variant="contained"
              color="primary"
              startIcon={<Add />}
              onClick={openAddDialog}
            >
              Add Java
            </Button>
          </Box>

          {javaInstallations.length === 0 ? (
            <Typography sx={{ color: '#9ca3af' }}>No Java installations found. Click "Add Java" to add one.</Typography>
          ) : (
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              {javaInstallations.map((java) => (
                <Box
                  key={java.id}
                  sx={{
                    backgroundColor: '#374151',
                    padding: 2,
                    borderRadius: 1,
                    display: 'flex',
                    flexDirection: 'column',
                    gap: 1
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                    <Tooltip title={java.is_default ? 'Default Java' : 'Set as default'}>
                      <IconButton
                        onClick={() => handleSetDefault(java.id)}
                        sx={{ color: java.is_default ? '#fbbf24' : '#9ca3af' }}
                      >
                        <Star />
                      </IconButton>
                    </Tooltip>
                    <div style={{ flex: 1 }}>
                      <Typography sx={{ color: 'white', fontWeight: 'bold' }}>
                        {java.friendly_name || `Java ${java.version}`}
                      </Typography>
                      <Typography sx={{ color: '#9ca3af', fontSize: '0.875rem' }}>
                        {java.version}
                      </Typography>
                    </div>
                    <Box sx={{ display: 'flex', gap: 1 }}>
                      <Tooltip title="Edit">
                        <IconButton onClick={() => openEditDialog(java)} sx={{ color: '#9ca3af' }}>
                          <Edit />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title="Delete">
                        <IconButton onClick={() => handleDeleteJava(java.id)} sx={{ color: '#ef4444' }}>
                          <Delete />
                        </IconButton>
                      </Tooltip>
                    </Box>
                  </Box>
                  <Chip
                    label={java.path}
                    sx={{
                      backgroundColor: '#1f2937',
                      color: '#d1d5db',
                      fontSize: '0.75rem',
                      maxWidth: '100%',
                      wordBreak: 'break-all'
                    }}
                  />
                </Box>
              ))}
            </Box>
          )}
        </CardContent>
      </Card>

      <Card sx={{ backgroundColor: '#1f2937', marginBottom: 2 }}>
        <CardContent>
          <Typography variant="h6" sx={{ color: 'white', marginBottom: 2 }}>
            Path Configuration
          </Typography>

          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <div>
              <Typography sx={{ color: '#9ca3af', fontSize: '0.875rem' }}>Java Path</Typography>
              {(() => {
                const defaultJava = javaInstallations.find(j => j.is_default);
                return defaultJava ? (
                  <Box sx={{ marginTop: 1 }}>
                    <Typography sx={{ color: 'white', fontWeight: 'bold' }}>
                      {defaultJava.friendly_name || `Java ${defaultJava.version}`}
                    </Typography>
                    <Typography sx={{ color: '#9ca3af', fontSize: '0.875rem' }}>
                      Version: {defaultJava.version}
                    </Typography>
                    <Chip
                      label={defaultJava.path}
                      sx={{
                        backgroundColor: '#1f2937',
                        color: '#d1d5db',
                        marginTop: 1,
                        maxWidth: '100%',
                        wordBreak: 'break-all',
                        fontSize: '0.75rem'
                      }}
                    />
                  </Box>
                ) : (
                  <Chip
                    label={config?.java_path || 'Not configured'}
                    sx={{
                      backgroundColor: '#374151',
                      color: 'white',
                      marginTop: 1,
                      maxWidth: '100%',
                      wordBreak: 'break-all'
                    }}
                  />
                );
              })()}
            </div>

            <div>
              <Typography sx={{ color: '#9ca3af', fontSize: '0.875rem' }}>Game Directory</Typography>
              <Chip
                label={config?.game_dir || 'Not configured'}
                sx={{
                  backgroundColor: '#374151',
                  color: 'white',
                  marginTop: 1,
                  maxWidth: '100%',
                  wordBreak: 'break-all'
                }}
              />
            </div>
          </Box>
        </CardContent>
      </Card>

      <Card sx={{ backgroundColor: '#1f2937' }}>
        <CardContent>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 2 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Memory sx={{ color: '#60a5fa' }} />
              <Typography variant="h6" sx={{ color: 'white' }}>
                Memory &amp; JVM Settings
              </Typography>
            </Box>
            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button
                onClick={handleResetConfig}
                disabled={!dirty || savingConfig}
                sx={{ color: '#9ca3af' }}
                startIcon={<X />}
              >
                Reset
              </Button>
              <Button
                onClick={handleSaveConfig}
                disabled={!dirty || savingConfig}
                variant="contained"
                color="primary"
                startIcon={<Save />}
              >
                {savingConfig ? 'Saving...' : 'Save'}
              </Button>
            </Box>
          </Box>

          {saveError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setSaveError('')}>
              {saveError}
            </Alert>
          )}

          <Typography sx={{ color: '#9ca3af', fontSize: '0.875rem', marginBottom: 1 }}>
            Maximum Memory: <Box component="span" sx={{ color: 'white', fontWeight: 'bold' }}>{maxMemory} MB</Box>
            <Box component="span" sx={{ color: '#9ca3af', marginLeft: 1 }}>(-Xmx{maxMemory}m)</Box>
          </Typography>
          <Slider
            value={maxMemory}
            min={MIN_MEMORY_MB}
            max={MAX_MEMORY_MB}
            step={256}
            marks={[
              { value: 1024, label: '1G' },
              { value: 2048, label: '2G' },
              { value: 4096, label: '4G' },
              { value: 8192, label: '8G' },
              { value: 16384, label: '16G' }
            ]}
            valueLabelDisplay="auto"
            valueLabelFormat={(v) => `${v} MB`}
            onChange={(_, value) => {
              setMaxMemory(value as number);
              setDirty(true);
            }}
            sx={{
              color: '#3b82f6',
              marginBottom: 2,
              '& .MuiSlider-markLabel': { color: '#9ca3af', fontSize: '0.75rem' }
            }}
          />
          <TextField
            type="number"
            label="Maximum Memory (MB)"
            variant="outlined"
            size="small"
            value={maxMemory}
            onChange={(e) => {
              const v = Number(e.target.value);
              if (!Number.isNaN(v)) {
                setMaxMemory(v);
                setDirty(true);
              }
            }}
            slotProps={{ htmlInput: { min: MIN_MEMORY_MB, max: MAX_MEMORY_MB, step: 256 } }}
            sx={{
              marginBottom: 3,
              width: 240,
              input: { color: 'white' },
              label: { color: '#9ca3af' },
              '& .MuiOutlinedInput-root': {
                '& fieldset': { borderColor: '#4b5563' },
                '&:hover fieldset': { borderColor: '#60a5fa' },
                '&.Mui-focused fieldset': { borderColor: '#3b82f6' }
              }
            }}
          />

          <Typography sx={{ color: '#9ca3af', fontSize: '0.875rem', marginBottom: 1 }}>
            Minimum Memory: <Box component="span" sx={{ color: 'white', fontWeight: 'bold' }}>{minMemory} MB</Box>
            <Box component="span" sx={{ color: '#9ca3af', marginLeft: 1 }}>(-Xms{minMemory}m)</Box>
          </Typography>
          <Slider
            value={Math.min(minMemory, maxMemory)}
            min={MIN_MEMORY_MB}
            max={maxMemory}
            step={256}
            marks={[
              { value: MIN_MEMORY_MB, label: `${MIN_MEMORY_MB}M` },
              { value: Math.round(maxMemory / 2), label: `${Math.round(maxMemory / 2)}M` },
              { value: maxMemory, label: `${maxMemory}M` }
            ]}
            valueLabelDisplay="auto"
            valueLabelFormat={(v) => `${v} MB`}
            onChange={(_, value) => {
              setMinMemory(value as number);
              setDirty(true);
            }}
            sx={{
              color: '#10b981',
              marginBottom: 2,
              '& .MuiSlider-markLabel': { color: '#9ca3af', fontSize: '0.75rem' }
            }}
          />
          <TextField
            type="number"
            label="Minimum Memory (MB)"
            variant="outlined"
            size="small"
            value={minMemory}
            onChange={(e) => {
              const v = Number(e.target.value);
              if (!Number.isNaN(v)) {
                setMinMemory(v);
                setDirty(true);
              }
            }}
            slotProps={{ htmlInput: { min: MIN_MEMORY_MB, max: MAX_MEMORY_MB, step: 256 } }}
            sx={{
              marginBottom: 3,
              width: 240,
              input: { color: 'white' },
              label: { color: '#9ca3af' },
              '& .MuiOutlinedInput-root': {
                '& fieldset': { borderColor: '#4b5563' },
                '&:hover fieldset': { borderColor: '#60a5fa' },
                '&.Mui-focused fieldset': { borderColor: '#3b82f6' }
              }
            }}
          />

          <Typography sx={{ color: '#9ca3af', fontSize: '0.875rem', marginBottom: 1 }}>
            Additional JVM Arguments
          </Typography>
          <TextField
            label="Java Args"
            variant="outlined"
            fullWidth
            multiline
            minRows={2}
            maxRows={4}
            value={javaArgs}
            onChange={(e) => {
              setJavaArgs(e.target.value);
              setDirty(true);
            }}
            placeholder="e.g., -XX:+UseG1GC -XX:MaxGCPauseMillis=50"
            sx={{
              marginBottom: 3,
              input: { color: 'white' },
              textarea: { color: 'white', fontFamily: 'monospace', fontSize: '0.875rem' },
              label: { color: '#9ca3af' },
              '& .MuiOutlinedInput-root': {
                '& fieldset': { borderColor: '#4b5563' },
                '&:hover fieldset': { borderColor: '#60a5fa' },
                '&.Mui-focused fieldset': { borderColor: '#3b82f6' }
              }
            }}
          />

          <Typography sx={{ color: '#9ca3af', fontSize: '0.875rem', marginBottom: 1 }}>
            Game Window Size
          </Typography>
          <Box sx={{ display: 'flex', gap: 2, marginBottom: 2 }}>
            <TextField
              type="number"
              label="Width"
              variant="outlined"
              size="small"
              value={gameWidth}
              onChange={(e) => {
                const v = Number(e.target.value);
                if (!Number.isNaN(v)) {
                  setGameWidth(v);
                  setDirty(true);
                }
              }}
              slotProps={{ htmlInput: { min: MIN_GAME_DIMENSION, max: MAX_GAME_DIMENSION, step: 1 } }}
              sx={{
                width: 140,
                input: { color: 'white' },
                label: { color: '#9ca3af' },
                '& .MuiOutlinedInput-root': {
                  '& fieldset': { borderColor: '#4b5563' },
                  '&:hover fieldset': { borderColor: '#60a5fa' },
                  '&.Mui-focused fieldset': { borderColor: '#3b82f6' }
                }
              }}
            />
            <TextField
              type="number"
              label="Height"
              variant="outlined"
              size="small"
              value={gameHeight}
              onChange={(e) => {
                const v = Number(e.target.value);
                if (!Number.isNaN(v)) {
                  setGameHeight(v);
                  setDirty(true);
                }
              }}
              slotProps={{ htmlInput: { min: MIN_GAME_DIMENSION, max: MAX_GAME_DIMENSION, step: 1 } }}
              sx={{
                width: 140,
                input: { color: 'white' },
                label: { color: '#9ca3af' },
                '& .MuiOutlinedInput-root': {
                  '& fieldset': { borderColor: '#4b5563' },
                  '&:hover fieldset': { borderColor: '#60a5fa' },
                  '&.Mui-focused fieldset': { borderColor: '#3b82f6' }
                }
              }}
            />
          </Box>

          <Typography sx={{ color: '#9ca3af', fontSize: '0.875rem', marginBottom: 1 }}>
            JVM Args Preview
          </Typography>
          <Chip
            label={
              `-Xms${minMemory}m -Xmx${maxMemory}m ` +
              (javaArgs.trim() ? `${javaArgs.trim()} ` : '') +
              `--width ${gameWidth} --height ${gameHeight}`
            }
            sx={{
              backgroundColor: '#1f2937',
              color: '#d1d5db',
              fontFamily: 'monospace',
              fontSize: '0.75rem',
              maxWidth: '100%',
              height: 'auto',
              whiteSpace: 'normal',
              padding: '8px 12px',
              wordBreak: 'break-all'
            }}
          />
        </CardContent>
      </Card>

      <Dialog open={openDialog} onClose={() => setOpenDialog(false)} maxWidth="md">
        <DialogTitle sx={{ backgroundColor: '#1f2937', color: 'white' }}>
          {editingJava ? 'Edit Java Installation' : 'Add Java Installation'}
        </DialogTitle>
        <DialogContent sx={{ backgroundColor: '#1f2937', color: 'white' }}>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, marginTop: 2 }}>
            <TextField
              label="Java Path"
              variant="outlined"
              fullWidth
              value={newJavaPath}
              onChange={(e) => setNewJavaPath(e.target.value)}
              placeholder="e.g., /usr/bin/java or C:\Program Files\Java\jdk17\bin\java.exe"
              sx={{
                input: { color: 'white' },
                label: { color: '#9ca3af' },
                '& .MuiOutlinedInput-root': {
                  '& fieldset': { borderColor: '#4b5563' },
                  '&:hover fieldset': { borderColor: '#60a5fa' },
                  '&.Mui-focused fieldset': { borderColor: '#3b82f6' }
                }
              }}
            />
            <TextField
              label="Friendly Name (Optional)"
              variant="outlined"
              fullWidth
              value={newJavaName}
              onChange={(e) => setNewJavaName(e.target.value)}
              placeholder="e.g., Java 17 (Forge)"
              sx={{
                input: { color: 'white' },
                label: { color: '#9ca3af' },
                '& .MuiOutlinedInput-root': {
                  '& fieldset': { borderColor: '#4b5563' },
                  '&:hover fieldset': { borderColor: '#60a5fa' },
                  '&.Mui-focused fieldset': { borderColor: '#3b82f6' }
                }
              }}
            />
            {dialogError && (
              <Typography sx={{ color: '#ef4444', fontSize: '0.875rem' }}>{dialogError}</Typography>
            )}
          </Box>
        </DialogContent>
        <DialogActions sx={{ backgroundColor: '#1f2937' }}>
          <Button
            onClick={() => {
              setOpenDialog(false);
              setEditingJava(null);
              setNewJavaPath('');
              setNewJavaName('');
              setDialogError('');
            }}
            sx={{ color: '#9ca3af' }}
            startIcon={<X />}
          >
            Cancel
          </Button>
          <Button
            onClick={editingJava ? handleUpdateJava : handleAddJava}
            variant="contained"
            color="primary"
            startIcon={<Check />}
          >
            {editingJava ? 'Update' : 'Add'}
          </Button>
        </DialogActions>
      </Dialog>

      <Snackbar
        open={saveSuccess}
        autoHideDuration={3000}
        onClose={() => setSaveSuccess(false)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
      >
        <Alert severity="success" onClose={() => setSaveSuccess(false)} sx={{ width: '100%' }}>
          Memory settings saved
        </Alert>
      </Snackbar>
    </Box>
  );
}