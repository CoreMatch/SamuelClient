import { useState } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { HRPAuthLogin } from '../../wailsjs/go/main/App';
import { t } from '../i18n';
import { Button, IconButton } from '../ui/button';
import { Select } from '../ui/select';
import { TextField, Field } from '../ui/field';
import { Callout } from '../ui/callout';

interface LoginViewProps {
  onLoginSuccess: (account: LoggedInAccount) => void;
  onSwitchToAccounts: () => void;
}

export interface LoggedInAccount {
  username: string;
  uuid: string;
  accessToken: string;
  clientToken: string;
  authType: 'offline' | 'hrpauth';
}

export default function LoginView({ onLoginSuccess, onSwitchToAccounts }: LoginViewProps) {
  const [authType, setAuthType] = useState<'hrpauth' | 'offline'>('hrpauth');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleAuthTypeChange = (type: string) => {
    setAuthType(type as 'hrpauth' | 'offline');
    setError('');
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
        // HRPAuth login (the only online provider; the endpoint is built in)
        if (!username.trim() || !password.trim()) {
          setError(t('login.credentialsRequired'));
          setLoading(false);
          return;
        }

        // ADR-0004: the plaintext password travels only through the in-process
        // Wails binding to the auth service — never over a local HTTP socket.
        const result = await HRPAuthLogin(username.trim(), password, '');

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
            authType: 'hrpauth',
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
    <div className="mx-auto mt-10 w-full max-w-[460px]">
      <div className="rounded-lg bg-panel p-10">
        <div className="flex flex-col gap-6">
          <Field label={t('login.authType')} htmlFor="login-auth-type">
            <Select
              ariaLabel={t('login.authType')}
              value={authType}
              onValueChange={handleAuthTypeChange}
              options={[
                { value: 'hrpauth', label: t('login.hrpauth') },
                { value: 'offline', label: t('login.offline') },
              ]}
            />
          </Field>

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

          {authType === 'hrpauth' && (
            <Field label={t('login.password')} htmlFor="login-password">
              <div className="relative">
                <input
                  id="login-password"
                  type={showPassword ? 'text' : 'password'}
                  value={password}
                  onChange={e => setPassword(e.target.value)}
                  autoComplete="current-password"
                  className="h-11 w-full rounded-md border border-edge bg-raised px-4 pr-12 text-base text-ink placeholder:text-ink-faint transition-colors duration-150 ease-standard hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong"
                />
                <IconButton
                  label={showPassword ? t('login.hidePassword') : t('login.showPassword')}
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-1.5 top-1/2 -translate-y-1/2"
                >
                  {showPassword ? <EyeOff className="size-5" /> : <Eye className="size-5" />}
                </IconButton>
              </div>
            </Field>
          )}

          <Button size="lg" className="mt-4 w-full" onClick={handleLogin} loading={loading}>
            {t('login.submit')}
          </Button>

          <div className="text-center">
            <Button variant="ghost" onClick={onSwitchToAccounts}>
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
