import { useState } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import { HRPAuthLogin } from '../../wailsjs/go/main/App';
import { t } from '../i18n';
import { Button, IconButton } from '../ui/button';
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
  // HRPAuth is the only login path (ADR-0008: offline login removed;
  // Microsoft login is designed in ADR-0007 and gated on Azure approval).
  authType: 'hrpauth';
}

export default function LoginView({ onLoginSuccess, onSwitchToAccounts }: LoginViewProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const handleLogin = async () => {
    setError('');
    setLoading(true);

    try {
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

// Generate random client token (fallback for the rare case where the auth
// response echoes no client token).
function generateClientToken(): string {
  const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  let result = '';
  for (let i = 0; i < 32; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}
