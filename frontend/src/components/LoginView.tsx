import { useRef, useState } from 'react';
import { Eye, EyeOff } from 'lucide-react';
import {
  HRPAuthLogin,
  MicrosoftBeginDeviceCode,
  MicrosoftCompleteDeviceCode,
} from '../../wailsjs/go/main/App';
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime';
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
  // HRPAuth is the primary login path (ADR-0008 removed offline login);
  // Microsoft login (ADR-0007) maps a licensed Minecraft account onto the
  // same HA account.
  authType: 'hrpauth' | 'microsoft';
  // Present only for Microsoft logins: the MSA refresh token that the app
  // shell persists to the OS keychain (90-day rolling, ADR-0007). It never
  // reaches localStorage.
  msaRefreshToken?: string;
}

// Device-code grant state shown while the Go side polls the MSA token
// endpoint (MicrosoftBeginDeviceCode -> MicrosoftCompleteDeviceCode).
interface DeviceCodeState {
  deviceCode: string;
  userCode: string;
  verificationUri: string;
  expiresIn: number;
  interval: number;
}

export default function LoginView({ onLoginSuccess, onSwitchToAccounts }: LoginViewProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  // Microsoft device-code flow (ADR-0007): null while the credential form is
  // shown, set once the device code is issued and the Go side starts polling.
  const [deviceCode, setDeviceCode] = useState<DeviceCodeState | null>(null);
  const [msBusy, setMsBusy] = useState(false);
  // Every begin/cancel bumps the flow id; a completing poll whose id no
  // longer matches was cancelled and its result is dropped.
  const msFlowRef = useRef(0);

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

  const resetMicrosoftFlow = () => {
    setDeviceCode(null);
    setMsBusy(false);
  };

  // Microsoft login (ADR-0007): show a device code, then let the Go side
  // walk MSA -> XBL -> XSTS -> Minecraft Services -> HA exchange while the
  // player completes the sign-in in a browser.
  const handleMicrosoftLogin = async () => {
    setError('');
    setMsBusy(true);
    const flow = ++msFlowRef.current;
    try {
      const begin = await MicrosoftBeginDeviceCode();
      if (!begin.success || !begin.data) {
        setError(begin.error || t('login.microsoft.failed'));
        setMsBusy(false);
        return;
      }
      const dc = begin.data as {
        device_code: string;
        user_code: string;
        verification_uri: string;
        expires_in: number;
        interval: number;
      };
      setDeviceCode({
        deviceCode: dc.device_code,
        userCode: dc.user_code,
        verificationUri: dc.verification_uri,
        expiresIn: dc.expires_in,
        interval: dc.interval,
      });

      const complete = await MicrosoftCompleteDeviceCode(dc.device_code, dc.expires_in, dc.interval);
      if (flow !== msFlowRef.current) return; // cancelled while polling
      resetMicrosoftFlow();

      if (complete.success && complete.data) {
        const data = complete.data as {
          accessToken?: string;
          clientToken?: string;
          msaRefreshToken?: string;
          uuid?: string;
          username?: string;
        };
        if (!data.uuid || !data.username) {
          setError(t('login.noProfile'));
          return;
        }
        onLoginSuccess({
          username: data.username,
          uuid: data.uuid,
          accessToken: data.accessToken ?? '',
          clientToken: data.clientToken || generateClientToken(),
          authType: 'microsoft',
          msaRefreshToken: data.msaRefreshToken,
        });
      } else {
        setError(complete.error || t('login.microsoft.failed'));
      }
    } catch {
      // A cancelled flow must not touch the UI again.
      if (flow === msFlowRef.current) {
        resetMicrosoftFlow();
        setError(t('login.microsoft.failed'));
      }
    }
  };

  const handleMicrosoftCancel = () => {
    msFlowRef.current++;
    resetMicrosoftFlow();
    setError('');
  };

  // Device-code stage: the player finishes the sign-in in a browser while
  // the Go side polls the MSA token endpoint.
  if (deviceCode) {
    return (
      <div className="mx-auto mt-10 w-full max-w-[460px]">
        <div className="rounded-lg bg-panel p-10">
          <div className="flex flex-col gap-6">
            {error && (
              <Callout tone="danger" onDismiss={() => setError('')}>
                {error}
              </Callout>
            )}

            <p className="text-sm text-ink-soft">{t('login.microsoft.codePrompt')}</p>

            <div
              aria-label={t('login.microsoft.codeLabel')}
              className="rounded-md border border-edge bg-raised px-4 py-5 text-center font-mono text-3xl font-semibold tracking-[0.3em] text-ink"
            >
              {deviceCode.userCode}
            </div>

            <div className="flex flex-col gap-2 text-sm text-ink-soft">
              <span>{deviceCode.verificationUri}</span>
              <Button variant="ghost" onClick={() => BrowserOpenURL(deviceCode.verificationUri)}>
                {t('login.microsoft.openPage')}
              </Button>
            </div>

            <p className="text-sm text-ink-faint">{t('login.microsoft.waiting')}</p>

            <Button variant="ghost" onClick={handleMicrosoftCancel}>
              {t('common.cancel')}
            </Button>
          </div>
        </div>
      </div>
    );
  }

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

          <div className="flex items-center gap-3 text-xs text-ink-faint">
            <div className="h-px flex-1 bg-edge" />
            {t('login.or')}
            <div className="h-px flex-1 bg-edge" />
          </div>

          <Button variant="secondary" className="w-full" onClick={handleMicrosoftLogin} loading={msBusy}>
            {t('login.microsoft.submit')}
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
