import { useState, useEffect } from 'react';
import { Asset, type AssetId } from '@ha/ui';
import HomeView from './components/HomeView';
import VersionManager from './components/VersionManager';
import ProfileView from './components/ProfileView';
import SettingsView from './components/SettingsView';
import HandoffBanner from './components/HandoffBanner';
import LoginView, { LoggedInAccount } from './components/LoginView';
import {
  StoredAccount,
  saveAccount,
  restoreLastUsedAccount,
  consumeOfflineRemovedNotice,
  clearStoredAccounts,
} from './accountStorage';
import { isPlayableVersion, type DiskVersion, type InstalledVersion, type HandoffPayload } from './types';
import { t } from './i18n';
import type { I18nKey } from './i18n';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { Callout } from './ui/callout';
import { TooltipProvider } from './ui/tooltip';
import DownloadQueue from './components/DownloadQueue';
import { UpdateDialog } from './components/UpdateDialog';
import { updateDownloadQueue } from './downloadQueue';
import type { DownloadProgressEvent, DownloadTask } from './downloadQueue';
import { useUpdateStatus } from './useUpdateStatus';
import {
  GetPendingHandoff,
  DismissHandoff,
  GetInstalledVersions,
  LaunchGame,
  KeychainStatus,
  CancelInstall,
} from '../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';

interface NavItem {
  id: string;
  labelKey: 'nav.play' | 'nav.versions' | 'nav.profile' | 'nav.settings';
  icon: AssetId;
}

const navItems: NavItem[] = [
  { id: 'home', labelKey: 'nav.play', icon: 'icon-play' },
  { id: 'versions', labelKey: 'nav.versions', icon: 'icon-version' },
  { id: 'profile', labelKey: 'nav.profile', icon: 'mode-avatar' },
  { id: 'settings', labelKey: 'nav.settings', icon: 'icon-settings' },
];

const VERSION_STORAGE_KEY = 'selected_version';

function readStoredVersion(): InstalledVersion | null {
  try {
    const raw = localStorage.getItem(VERSION_STORAGE_KEY);
    if (!raw) return null;
    const version = JSON.parse(raw) as InstalledVersion;
    return version && typeof version.id === 'string' ? version : null;
  } catch {
    return null;
  }
}

export default function App() {
  const [selectedView, setSelectedView] = useState<string>('home');
  const [selectedVersion, setSelectedVersion] = useState<InstalledVersion | null>(readStoredVersion);
  const [currentAccount, setCurrentAccount] = useState<StoredAccount | null>(null);
  const [showLogin, setShowLogin] = useState(false);
  const [downloadTasks, setDownloadTasks] = useState<DownloadTask[]>([]);

  // Match handoff state (INTEROP-SPEC): pending payload, readable
  // degradation notice, join progress and failure text.
  const [handoff, setHandoff] = useState<HandoffPayload | null>(null);
  const [handoffNotice, setHandoffNotice] = useState('');
  const [joiningMatch, setJoiningMatch] = useState(false);
  const [joinError, setJoinError] = useState('');

  // One-shot notices (ADR-0008): offline accounts removed by the storage
  // migration, keychain availability, and token-storage warnings.
  const [offlineRemoved, setOfflineRemoved] = useState<string[] | null>(null);
  const [keychainError, setKeychainError] = useState('');
  const [keychainFallback, setKeychainFallback] = useState(false);
  const [tokenStorageWarning, setTokenStorageWarning] = useState('');
  const update = useUpdateStatus();

  useEffect(() => {
    // Restore the last-used account. HRPAuth tokens are validated
    // silently and refreshed when stale; if both fail the login view opens so
    // the user is prompted to sign in again (PENDING-DECISIONS #11).
    restoreLastUsedAccount().then(({ account, needsReLogin, keychainError: migrationError }) => {
      setCurrentAccount(account);
      if (needsReLogin) {
        setShowLogin(true);
      }
      if (migrationError) {
        setKeychainError(migrationError);
      }
    });

    // Surface the one-shot offline-removal notice left by the migration and
    // probe the OS credential store once; both degrade readably instead of
    // failing silently (ADR-0008).
    setOfflineRemoved(consumeOfflineRemovedNotice());
    KeychainStatus()
      .then(result => {
        if (!result.success || !result.data) return;
        if (result.data.backend === 'file') {
          // Automatic keychain-to-file fallback (ADR-0010): informational,
          // not an error — sessions persist in tokens.json.
          setKeychainFallback(true);
        } else if (result.data.available === false) {
          setKeychainError(String(result.data.error || 'unknown error'));
        }
      })
      .catch(() => {});
  }, []);

  // Keep the queue listener in the app shell so it remains mounted while
  // page-level views are replaced during navigation.
  useEffect(() => EventsOn('download:progress', (payload: DownloadProgressEvent) => {
    setDownloadTasks(current => updateDownloadQueue(current, payload));
  }), []);

  // Re-hydrate a pending handoff (survives reload) and follow backend
  // lifecycle events (INTEROP-SPEC §6/§7).
  useEffect(() => {
    GetPendingHandoff()
      .then(result => {
        if (result.success && result.data?.status === 'pending' && result.data.handoff) {
          setHandoff(result.data.handoff as HandoffPayload);
        }
      })
      .catch(() => {});

    EventsOn('interop:handoff', (payload: { status?: string; handoff?: HandoffPayload; message?: string }) => {
      if (!payload || !payload.status) return;
      if (payload.status === 'pending') {
        if (payload.handoff) {
          setHandoff(payload.handoff);
          setJoinError('');
          setHandoffNotice('');
        }
        return;
      }
      if (payload.status === 'dismissed') {
        setHandoff(null);
        setJoinError('');
        return;
      }
      // gone / pull_timeout / version_mismatch / invalid: readable
      // degradation, never a silent drop.
      setHandoff(null);
      setJoinError('');
      if (payload.message) {
        setHandoffNotice(payload.message);
      }
    });
    return () => {
      EventsOff('interop:handoff');
    };
  }, []);

  // Launch the pending handoff's match with the given account: pick an
  // installed version (game_version hint > current selection > first
  // complete install) and go straight into the match server. Failures keep
  // the handoff pending so the result is not lost.
  const joinHandoff = async (account: StoredAccount) => {
    if (!handoff) return;
    // Tokens are hydrated from the keychain; a session without them cannot
    // join — send the user to sign in instead of launching a doomed game.
    if (!account.accessToken || !account.clientToken) {
      setShowLogin(true);
      return;
    }
    setJoiningMatch(true);
    setJoinError('');
    try {
      let complete: DiskVersion[] = [];
      try {
        const versionsResult = await GetInstalledVersions();
        if (versionsResult.success && Array.isArray(versionsResult.data?.versions)) {
          complete = (versionsResult.data.versions as DiskVersion[]).filter(isPlayableVersion);
        }
      } catch {
        // fall through: reported as "no version" below
      }

      const hint = handoff.match.join_target.game_version;
      const selectedCopy = selectedVersion && complete.find(v =>
        v.id === selectedVersion.id && (!selectedVersion.gameDir || v.game_dir === selectedVersion.gameDir));
      const chosen =
        (hint && selectedCopy?.id === hint && selectedCopy) ||
        (hint && complete.find(v => v.id === hint)) ||
        selectedCopy ||
        complete[0];
      if (!chosen) {
        setJoinError(t('handoff.noVersion'));
        return;
      }

      const result = await LaunchGame({
        version_id: chosen.id,
        version_type: chosen.type,
        username: account.username,
        uuid: account.uuid,
        access_token: account.accessToken,
        client_token: account.clientToken,
        auth_type: account.authType,
        authlib_jar: '',
        authlib_meta: '',
        game_dir: chosen.game_dir ?? '',
        java_path: '',
        loader_type: '',
        loader_version: '',
        max_memory: 0,
        min_memory: 0,
        java_args: '',
        game_width: 0,
        game_height: 0,
        join_server: handoff.match.join_target.host,
        join_port: handoff.match.join_target.port,
      });

      if (result.success) {
        await DismissHandoff().catch(() => {});
        setHandoff(null);
        setShowLogin(false);
        setSelectedView('home');
      } else {
        setJoinError(t('handoff.launchFailed', { error: result.error || 'unknown error' }));
      }
    } catch {
      setJoinError(t('handoff.launchFailed', { error: 'unexpected failure' }));
    } finally {
      setJoiningMatch(false);
    }
  };

  const handleHandoffJoin = () => {
    if (!currentAccount) {
      setShowLogin(true);
      return;
    }
    joinHandoff(currentAccount);
  };

  const handleHandoffDismiss = () => {
    DismissHandoff().catch(() => {});
    setHandoff(null);
    setHandoffNotice('');
    setJoinError('');
  };

  const handleLoginSuccess = async (account: LoggedInAccount) => {
    const { account: savedAccount, tokenStorageError } = await saveAccount(account);
    if (tokenStorageError) {
      setTokenStorageWarning(tokenStorageError);
    } else {
      setTokenStorageWarning('');
    }
    setCurrentAccount(savedAccount);
    setShowLogin(false);
    setSelectedView('home');
    // Auto-resume a pending match handoff after login (INTEROP-SPEC §6).
    if (handoff) {
      joinHandoff(savedAccount);
    }
  };

  // The selected version survives restarts (localStorage metadata only).
  const handleVersionSelect = (version: InstalledVersion | null) => {
    setSelectedVersion(version);
    try {
      if (version) {
        localStorage.setItem(VERSION_STORAGE_KEY, JSON.stringify(version));
      } else {
        localStorage.removeItem(VERSION_STORAGE_KEY);
      }
    } catch {
      // Storage unavailable: the selection still works for this session.
    }
  };

  const handleSignOut = async () => {
    await clearStoredAccounts(currentAccount ? [currentAccount.id] : []);
    setCurrentAccount(null);
    setTokenStorageWarning('');
    setShowLogin(false);
    setSelectedView('profile');
  };

  const handleOpenSettings = () => {
    setSelectedView('settings');
  };

  const renderView = () => {
    if (showLogin) {
      return (
        <LoginView
          onLoginSuccess={handleLoginSuccess}
        />
      );
    }

    switch (selectedView) {
      case 'home':
        return (
          <HomeView
            selectedVersion={selectedVersion}
            onVersionSelect={handleVersionSelect}
            currentAccount={currentAccount}
            onOpenSettings={handleOpenSettings}
          />
        );
      case 'versions':
        return <VersionManager onSelectVersion={handleVersionSelect} />;
      case 'profile':
        return (
          <ProfileView
            account={currentAccount}
            onSignIn={() => setShowLogin(true)}
            onSignOut={handleSignOut}
          />
        );
      case 'settings':
        return <SettingsView />;
      default:
        return (
          <HomeView
            selectedVersion={selectedVersion}
            onVersionSelect={handleVersionSelect}
            currentAccount={currentAccount}
            onOpenSettings={handleOpenSettings}
          />
        );
    }
  };

  const getCurrentTitle = (): I18nKey => {
    if (showLogin) return 'login.title';
    const item = navItems.find(n => n.id === selectedView);
    return item ? item.labelKey : 'nav.play';
  };

  const updateDialog = (
    <UpdateDialog
      status={update.status}
      open={update.visible}
      onInstall={() => { void update.install(); }}
      onDismiss={update.dismiss}
      onRetry={() => { void update.retry(); }}
    />
  );

  if (update.blocking) {
    return (
      <TooltipProvider>
        <div className="grid h-screen place-items-center bg-base p-4 text-ink">
          {updateDialog}
        </div>
      </TooltipProvider>
    );
  }

  return (
    <TooltipProvider>
      <div className="flex h-screen overflow-hidden bg-base text-ink">
        {/* Persistent sidebar */}
        <aside className="flex w-[248px] shrink-0 flex-col border-r border-edge bg-panel">
          <div className="px-6 pb-8 pt-8">
            <div className="flex items-center gap-3">
              <Asset id="brand-samuel" size={32} className="text-accent" title="Samuel" />
              <span className="text-base font-semibold text-ink">Samuel</span>
            </div>
          </div>

          <nav className="flex flex-col gap-1.5 px-4" aria-label="Primary">
            {navItems.map(item => {
              const active = selectedView === item.id && !showLogin;
              return (
                <button
                  key={item.id}
                  type="button"
                  aria-current={active ? 'page' : undefined}
                  onClick={() => {
                    setSelectedView(item.id);
                    setShowLogin(false);
                  }}
                  className={`flex h-10 w-full items-center gap-3 rounded-md px-3.5 text-base font-medium transition-colors duration-150 ease-standard ${
                    active ? 'bg-white/7 text-ink' : 'text-ink-muted hover:bg-white/4 hover:text-ink'
                  }`}
                >
                  <Asset id={item.icon} size={20} className={active ? 'text-accent' : ''} />
                  {t(item.labelKey)}
                </button>
              );
            })}
          </nav>

          <DownloadQueue
            tasks={downloadTasks}
            onCancel={versionId => { void CancelInstall(versionId).catch(() => {}); }}
            onDismiss={versionId => setDownloadTasks(current => current.filter(task => task.versionId !== versionId))}
          />

          <div className="mt-auto p-4">
            {currentAccount ? (
              <button
                type="button"
                onClick={() => {
                  setSelectedView('profile');
                  setShowLogin(false);
                }}
                className="flex w-full items-center gap-3 rounded-md border border-edge bg-raised p-3 text-left transition-colors duration-150 hover:border-edge-strong"
              >
                <Asset id="mode-avatar" size={36} className="text-ink-muted" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-base font-medium text-ink">
                    {currentAccount.username}
                  </span>
                  <Badge tone="success" className="mt-1">
                    {t(currentAccount.authType === 'microsoft' ? 'profile.microsoft' : 'profile.hrpauth')}
                  </Badge>
                </span>
              </button>
            ) : (
              <Button variant="secondary" className="w-full" onClick={() => setShowLogin(true)}>
                {t('nav.signIn')}
              </Button>
            )}
            {/* Usage-guidelines disclaimer: this product must not appear
                official or approved (ADR-0008). */}
            <p className="mt-3 text-[10px] leading-snug text-ink-faint">{t('app.disclaimer')}</p>
          </div>
        </aside>

        {/* Single-column main content */}
        <main className="flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[980px] px-14 py-12">
            <header className="mb-10">
              <h1 className="text-display font-semibold tracking-tight text-ink">{t(getCurrentTitle())}</h1>
            </header>
            {offlineRemoved && offlineRemoved.length > 0 && (
              <div className="mb-4">
                <Callout tone="warning" onDismiss={() => setOfflineRemoved(null)}>
                  {t('app.offlineRemoved', { usernames: offlineRemoved.join(', ') })}
                </Callout>
              </div>
            )}
            {keychainError && (
              <div className="mb-4">
                <Callout tone="danger">{t('app.keychainUnavailable', { error: keychainError })}</Callout>
              </div>
            )}
            {keychainFallback && (
              <div className="mb-4">
                <Callout tone="info" onDismiss={() => setKeychainFallback(false)}>
                  {t('app.keychainFallback')}
                </Callout>
              </div>
            )}
            {tokenStorageWarning && (
              <div className="mb-4">
                <Callout tone="warning" onDismiss={() => setTokenStorageWarning('')}>
                  {t('app.tokenStorageWarning', { error: tokenStorageWarning })}
                </Callout>
              </div>
            )}
            <HandoffBanner
              handoff={handoff}
              notice={handoffNotice}
              hasAccount={!!currentAccount}
              joining={joiningMatch}
              error={joinError}
              onJoin={handleHandoffJoin}
              onLogin={() => setShowLogin(true)}
              onDismiss={handleHandoffDismiss}
            />
            {renderView()}
          </div>
        </main>
      </div>
      {updateDialog}
    </TooltipProvider>
  );
}
