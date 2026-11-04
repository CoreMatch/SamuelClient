import { useState } from 'react';
import {
  Box,
  Paper,
  TextField,
  Button,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  CircularProgress,
  Alert,
  Collapse,
  IconButton,
  InputAdornment,
} from '@mui/material';
import {
  Visibility,
  VisibilityOff,
  Person,
  Lock,
  Http,
  Refresh,
} from '@mui/icons-material';
import { GetAuthlibMeta } from '../../wailsjs/go/main/App';

interface AuthServer {
  id: string;
  name: string;
  url: string;
  icon?: string;
}

interface LoginViewProps {
  onLoginSuccess: (account: LoggedInAccount) => void;
  onSwitchToAccounts: () => void;
}

export interface LoggedInAccount {
  username: string;
  uuid: string;
  accessToken: string;
  clientToken: string;
  authType: 'offline' | 'authlib-injector';
  authServer?: string;
}

const DEFAULT_AUTH_SERVERS: AuthServer[] = [
  { id: 'HRPAUTH', name: 'HRPAUTH', url: 'https://backend.auth.samuelcheston.com/' },
];

const DEFAULT_HRPAUTH_URL = 'https://backend.auth.samuelcheston.com/';

export default function LoginView({ onLoginSuccess, onSwitchToAccounts }: LoginViewProps) {
  const [authType, setAuthType] = useState<'authlib-injector' | 'offline'>('authlib-injector');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [authServer, setAuthServer] = useState(DEFAULT_HRPAUTH_URL);
  const [customAuthServer, setCustomAuthServer] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [authServers, setAuthServers] = useState<AuthServer[]>(DEFAULT_AUTH_SERVERS);
  const [loadingServers, setLoadingServers] = useState(false);

  const handleAuthTypeChange = (type: 'offline' | 'authlib-injector') => {
    setAuthType(type);
    setError('');
    if (type === 'offline') {
      setAuthServer('offline');
    } else {
      // Auto-select HRPAUTH as default when switching to authlib-injector
      setAuthServer(DEFAULT_HRPAUTH_URL);
    }
  };

  const handleAuthServerChange = (serverId: string) => {
    setAuthServer(serverId);
    setError('');
    if (serverId === 'custom') {
      setCustomAuthServer('');
    }
  };

  const loadAuthServers = async (serverUrl: string) => {
    setLoadingServers(true);
    try {
      // ADR-0004: Wails binding instead of the old (dev-only) relative fetch
      // `/api/v1/auth/meta?server=...`. The metadata document nests the server
      // name under `meta` (see AuthService.GetAuthlibMeta).
      const result = await GetAuthlibMeta(serverUrl);
      if (result.success && result.data) {
        const newServer: AuthServer = {
          id: serverUrl,
          name: result.data.meta?.serverName || serverUrl,
          url: serverUrl,
        };
        setAuthServers(prev => {
          const filtered = prev.filter(s => s.id !== 'custom');
          return [...filtered, newServer, { id: 'custom', name: 'Custom Server', url: 'custom' }];
        });
        setAuthServer(serverUrl);
      } else {
        setError(result.error || 'Failed to fetch auth server info');
      }
    } catch (err) {
      setError('Failed to connect to auth server');
    } finally {
      setLoadingServers(false);
    }
  };

  const handleAddCustomServer = () => {
    if (customAuthServer.trim()) {
      let serverUrl = customAuthServer.trim();
      if (!serverUrl.startsWith('http://') && !serverUrl.startsWith('https://')) {
        serverUrl = 'https://' + serverUrl;
      }
      loadAuthServers(serverUrl);
    }
  };

  const handleLogin = async () => {
    setError('');
    setLoading(true);

    try {
      if (authType === 'offline') {
        // Offline mode login
        if (!username.trim()) {
          setError('Username is required');
          setLoading(false);
          return;
        }
        // Generate offline UUID
        const offlineUUID = generateOfflineUUID(username);
        const clientToken = generateClientToken();

        onLoginSuccess({
          username: username.trim(),
          uuid: offlineUUID,
          accessToken: offlineUUID,
          clientToken: clientToken,
          authType: 'offline',
        });
      } else {
        // Authlib-injector login
        if (!authServer || authServer === 'custom' || !authServer.trim()) {
          setError('Please select or add an auth server');
          setLoading(false);
          return;
        }
        if (!username.trim() || !password.trim()) {
          setError('Username and password are required');
          setLoading(false);
          return;
        }

        const serverUrl = authServer;
        const response = await fetch('http://localhost:34501/api/v1/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            server: serverUrl,
            username: username.trim(),
            password: password,
          }),
        });

        const data = await response.json();
        if (data.success && data.data) {
          const profile = data.data.selectedProfile || data.data.availableProfiles?.[0];
          if (!profile) {
            setError('No profile found');
            setLoading(false);
            return;
          }
          onLoginSuccess({
            username: profile.name,
            uuid: profile.id,
            accessToken: data.data.accessToken,
            clientToken: data.data.clientToken || generateClientToken(),
            authType: 'authlib-injector',
            authServer: serverUrl,
          });
        } else {
          setError(data.error || 'Login failed');
        }
      }
    } catch (err) {
      setError('Network error. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Box
      sx={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'linear-gradient(135deg, #1a1d21 0%, #2d3339 100%)',
        p: 2,
      }}
    >
      <Paper
        elevation={6}
        sx={{
          p: 4,
          maxWidth: 420,
          width: '100%',
          backgroundColor: '#282c34',
          borderRadius: 2,
        }}
      >
        <Box sx={{ textAlign: 'center', mb: 3 }}>
        </Box>

        <Box sx={{ mb: 3 }}>
          <FormControl fullWidth sx={{ mb: 2 }}>
            <InputLabel sx={{ color: '#9ca3af' }}>Authentication Type</InputLabel>
            <Select
              value={authType}
              label="Authentication Type"
              onChange={(e) => handleAuthTypeChange(e.target.value as 'offline' | 'authlib-injector')}
              sx={{
                color: 'white',
                '.MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
                '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#4CAF50' },
                '.MuiSvgIcon-root': { color: '#9ca3af' },
              }}
            >
              <MenuItem value="authlib-injector">Yggdrasil API</MenuItem>
              <MenuItem value="offline">Offline (Cracked)</MenuItem>
            </Select>
          </FormControl>

          {authType === 'authlib-injector' && (
            <>
              <FormControl fullWidth sx={{ mb: 2 }}>
                <InputLabel sx={{ color: '#9ca3af' }}>Auth Server</InputLabel>
                <Select
                  value={authServer}
                  label="Auth Server"
                  onChange={(e) => handleAuthServerChange(e.target.value)}
                  sx={{
                    color: 'white',
                    '.MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
                    '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#4CAF50' },
                    '.MuiSvgIcon-root': { color: '#9ca3af' },
                  }}
                >
                  {authServers.map((server) => (
                    <MenuItem key={server.id} value={server.url || server.id}>
                      {server.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>

              {authServer === 'custom' && (
                <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
                  <TextField
                    fullWidth
                    label="Custom Server URL"
                    value={customAuthServer}
                    onChange={(e) => setCustomAuthServer(e.target.value)}
                    placeholder="https://auth.example.com"
                    size="small"
                    slotProps={{
                      input: {
                        startAdornment: (
                          <InputAdornment position="start">
                            <Http sx={{ color: '#9ca3af' }} />
                          </InputAdornment>
                        ),
                      },
                    }}
                    sx={{
                      '& .MuiInputBase-input': { color: 'white' },
                      '& .MuiInputLabel-root': { color: '#9ca3af' },
                      '& .MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
                    }}
                  />
                  <Button
                    variant="contained"
                    onClick={handleAddCustomServer}
                    disabled={loadingServers || !customAuthServer.trim()}
                    sx={{ minWidth: 'auto', backgroundColor: '#4CAF50' }}
                  >
                    {loadingServers ? <CircularProgress size={20} color="inherit" /> : <Refresh />}
                  </Button>
                </Box>
              )}
            </>
          )}
        </Box>

        <Collapse in={!!error}>
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>
            {error}
          </Alert>
        </Collapse>

        <TextField
          fullWidth
          label="Username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          margin="normal"
          slotProps={{
            input: {
              startAdornment: (
                <InputAdornment position="start">
                  <Person sx={{ color: '#9ca3af' }} />
                </InputAdornment>
              ),
            },
          }}
          sx={{
            '& .MuiInputBase-input': { color: 'white' },
            '& .MuiInputLabel-root': { color: '#9ca3af' },
            '& .MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
            '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#4CAF50' },
          }}
        />

        {authType === 'authlib-injector' && (
          <TextField
            fullWidth
            label="Password"
            type={showPassword ? 'text' : 'password'}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            margin="normal"
          slotProps={{
            input: {
              startAdornment: (
                <InputAdornment position="start">
                  <Lock sx={{ color: '#9ca3af' }} />
                </InputAdornment>
              ),
              endAdornment: (
                <InputAdornment position="end">
                  <IconButton onClick={() => setShowPassword(!showPassword)} edge="end">
                    {showPassword ? <VisibilityOff sx={{ color: '#9ca3af' }} /> : <Visibility sx={{ color: '#9ca3af' }} />}
                  </IconButton>
                </InputAdornment>
              ),
            },
          }}
            sx={{
              '& .MuiInputBase-input': { color: 'white' },
              '& .MuiInputLabel-root': { color: '#9ca3af' },
              '& .MuiOutlinedInput-notchedOutline': { borderColor: '#4a5568' },
              '&:hover .MuiOutlinedInput-notchedOutline': { borderColor: '#4CAF50' },
            }}
          />
        )}

        <Button
          fullWidth
          variant="contained"
          onClick={handleLogin}
          disabled={loading}
          sx={{
            mt: 3,
            mb: 2,
            py: 1.5,
            backgroundColor: '#4CAF50',
            fontSize: '1.1rem',
            '&:hover': { backgroundColor: '#45a049' },
          }}
        >
          {loading ? <CircularProgress size={24} color="inherit" /> : 'Login'}
        </Button>

        <Box sx={{ textAlign: 'center' }}>
          <Button
            variant="text"
            onClick={onSwitchToAccounts}
            sx={{ color: '#9ca3af', textTransform: 'none' }}
          >
            Manage Accounts
          </Button>
        </Box>
      </Paper>
    </Box>
  );
}

// Generate offline UUID from username
function generateOfflineUUID(username: string): string {
  let hash = 0;
  for (let i = 0; i < username.length; i++) {
    const char = username.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash;
  }

  const uuid = [
    (hash >>> 0).toString(16).padStart(8, '0'),
    ((hash >>> 16) & 0xFFFF).toString(16).padStart(4, '0'),
    ((hash >>> 16) & 0xFFFF).toString(16).padStart(4, '0'),
    ((hash >>> 16) & 0xFFFF).toString(16).padStart(4, '0'),
    Math.abs(hash).toString(16).padStart(12, '0').slice(0, 12),
  ].join('-');

  return uuid;
}

// Generate random client token
function generateClientToken(): string {
  const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  let result = '';
  for (let i = 0; i < 32; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}
