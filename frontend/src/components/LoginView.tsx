import { useState } from 'react';
import { Eye, EyeOff, RefreshCw } from 'lucide-react';
import { GetAuthlibMeta, AuthlibLogin } from '../../wailsjs/go/main/App';
import { t } from '../i18n';
import { Button, IconButton } from '../ui/button';
import { Select } from '../ui/select';
import { TextField, Field } from '../ui/field';
import { Callout } from '../ui/callout';

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

  const handleAuthTypeChange = (type: string) => {
    const next = type as 'offline' | 'authlib-injector';
    setAuthType(next);
    setError('');
    if (next === 'offline') {
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
          return [...filtered, newServer, { id: 'custom', name: t('login.customServer'), url: 'custom' }];
        });
        setAuthServer(serverUrl);
      } else {
        setError(result.error || t('login.metaFailed'));
      }
    } catch {
      setError(t('login.metaConnect'));
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
          setError(t('login.usernameRequired'));
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
          setError(t('login.selectServer'));
          setLoading(false);
          return;
        }
        if (!username.trim() || !password.trim()) {
          setError(t('login.credentialsRequired'));
          setLoading(false);
          return;
        }

        const serverUrl = authServer;
        // ADR-0004: the plaintext password travels only through the in-process
        // Wails binding to the auth service — never over a local HTTP socket.
        const result = await AuthlibLogin(serverUrl, username.trim(), password, '');

        if (result.success && result.data) {
          const profile = result.data.selectedProfile || result.data.availableProfiles?.[0];
          if (!profile) {
            setError(t('login.noProfile'));
            setLoading(false);
            return;
          }
          onLoginSuccess({
            username: profile.name,
            uuid: profile.id,
            accessToken: result.data.accessToken,
            clientToken: result.data.clientToken || generateClientToken(),
            authType: 'authlib-injector',
            authServer: serverUrl,
          });
        } else {
          setError(result.error || t('login.failed'));
        }
      }
    } catch {
      setError(t('login.failedRetry'));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="mx-auto mt-14 w-full max-w-[400px]">
      <div className="rounded-lg border border-edge bg-panel p-8">
        <h1 className="text-xl font-semibold text-ink">{t('login.title')}</h1>

        <div className="mt-6 flex flex-col gap-4">
          <Field label={t('login.authType')} htmlFor="login-auth-type">
            <Select
              ariaLabel={t('login.authType')}
              value={authType}
              onValueChange={handleAuthTypeChange}
              options={[
                { value: 'authlib-injector', label: t('login.yggdrasil') },
                { value: 'offline', label: t('login.offline') },
              ]}
            />
          </Field>

          {authType === 'authlib-injector' && (
            <>
              <Field label={t('login.authServer')} htmlFor="login-auth-server">
                <Select
                  ariaLabel={t('login.authServer')}
                  value={authServer}
                  onValueChange={handleAuthServerChange}
                  options={authServers.map(server => ({
                    value: server.url || server.id,
                    label: server.name,
                  }))}
                />
              </Field>

              {authServer === 'custom' && (
                <div className="flex items-end gap-2">
                  <TextField
                    className="flex-1"
                    label={t('login.customUrl')}
                    value={customAuthServer}
                    onChange={e => setCustomAuthServer(e.target.value)}
                    placeholder="https://auth.example.com"
                  />
                  <Button
                    variant="secondary"
                    onClick={handleAddCustomServer}
                    disabled={loadingServers || !customAuthServer.trim()}
                    loading={loadingServers}
                    aria-label={t('login.addServer')}
                    className="shrink-0"
                  >
                    {!loadingServers && <RefreshCw className="size-4" aria-hidden />}
                  </Button>
                </div>
              )}
            </>
          )}

          {error && (
            <Callout tone="danger" onDismiss={() => setError('')}>
              {error}
            </Callout>
          )}

          <TextField
            label={t('login.username')}
            value={username}
            onChange={e => setUsername(e.target.value)}
            autoComplete="username"
          />

          {authType === 'authlib-injector' && (
            <Field label={t('login.password')} htmlFor="login-password">
              <div className="relative">
                <input
                  id="login-password"
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  autoComplete="current-password"
                  className="h-9 w-full rounded-md border border-edge bg-raised px-3 pr-10 text-sm text-ink placeholder:text-ink-faint transition-colors duration-150 ease-standard hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong"
                />
                <IconButton
                  label={showPassword ? t('login.hidePassword') : t('login.showPassword')}
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-1 top-1/2 -translate-y-1/2"
                >
                  {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                </IconButton>
              </div>
            </Field>
          )}

          <Button size="lg" className="mt-2 w-full" onClick={handleLogin} loading={loading}>
            {t('login.submit')}
          </Button>

          <div className="text-center">
            <Button variant="ghost" size="sm" onClick={onSwitchToAccounts}>
              {t('login.manageAccounts')}
            </Button>
          </div>
        </div>
      </div>
    </div>
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
