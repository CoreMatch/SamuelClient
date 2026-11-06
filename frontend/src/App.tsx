import { useState, useEffect } from 'react';
import { Package, Play, Settings, Users } from 'lucide-react';
import HomeView from './components/HomeView';
import VersionManager from './components/VersionManager';
import AccountManager from './components/AccountManager';
import SettingsView from './components/SettingsView';
import HandoffBanner from './components/HandoffBanner';
import LoginView, { LoggedInAccount } from './components/LoginView';
import { StoredAccount, saveAccount, restoreLastUsedAccount } from './accountStorage';
import { InstalledVersion, HandoffPayload } from './types';
import { t } from './i18n';
import type { I18nKey } from './i18n';
import { Badge } from './ui/badge';
import { Button } from './ui/button';
import { TooltipProvider } from './ui/tooltip';
import {
  GetPendingHandoff,
  DismissHandoff,
  GetInstalledVersions,
  LaunchGame,
} from '../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';

interface NavItem {
  id: string;
  labelKey: 'nav.play' | 'nav.versions' | 'nav.accounts' | 'nav.settings';
  icon: typeof Play;
}

const navItems: NavItem[] = [
  { id: 'home', labelKey: 'nav.play', icon: Play },
  { id: 'versions', labelKey: 'nav.versions', icon: Package },
  { id: 'accounts', labelKey: 'nav.accounts', icon: Users },
  { id: 'settings', labelKey: 'nav.settings', icon: Settings },
];

export default function App() {
  const [selectedView, setSelectedView] = useState<string>('home');
  const [selectedVersion, setSelectedVersion] = useState<InstalledVersion | null>(null);
  const [currentAccount, setCurrentAccount] = useState<StoredAccount | null>(null);
  const [showLogin, setShowLogin] = useState(false);

  // Match handoff state (INTEROP-SPEC): pending payload, readable
  // degradation notice, join progress and failure text.
  const [handoff, setHandoff] = useState<HandoffPayload | null>(null);
  const [handoffNotice, setHandoffNotice] = useState('');
  const [joiningMatch, setJoiningMatch] = useState(false);
  const [joinError, setJoinError] = useState('');

  useEffect(() => {
    // Restore the last-used account. HRPAuth tokens are validated
    // silently and refreshed when stale; if both fail the login view opens so
    // the user is prompted to sign in again (PENDING-DECISIONS #11).
    restoreLastUsedAccount().then(({ account, needsReLogin }) => {
      setCurrentAccount(account);
      if (needsReLogin) {
        setShowLogin(true);
      }
    });
  }, []);

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
    setJoiningMatch(true);
    setJoinError('');
    try {
      interface BackendVersion {
        id: string;
        type: string;
        has_json: boolean;
        has_jar: boolean;
      }
      let complete: BackendVersion[] = [];
      try {
        const versionsResult = await GetInstalledVersions();
        if (versionsResult.success && Array.isArray(versionsResult.data?.versions)) {
          complete = (versionsResult.data.versions as BackendVersion[]).filter(v => v.has_json && v.has_jar);
        }
      } catch {
        // fall through: reported as "no version" below
      }

      const hint = handoff.match.join_target.game_version;
      const chosen =
        (hint && complete.find(v => v.id === hint)) ||
        (selectedVersion && complete.find(v => v.id === selectedVersion.id)) ||
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
        authlib_jar: localStorage.getItem('authlib_jar_path') || '',
        authlib_meta: '',
        game_dir: '',
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

  const handleLoginSuccess = (account: LoggedInAccount) => {
    const savedAccount = saveAccount(account);
    setCurrentAccount(savedAccount);
    setShowLogin(false);
    setSelectedView('home');
    // Auto-resume a pending match handoff after login (INTEROP-SPEC §6).
    if (handoff) {
      joinHandoff(savedAccount);
    }
  };

  const handleSelectAccount = (account: StoredAccount) => {
    setCurrentAccount(account);
    setSelectedView('home');
  };

  const handleAddAccount = () => {
    setShowLogin(true);
  };

  const handleOpenSettings = () => {
    setSelectedView('settings');
  };

  const renderView = () => {
    if (showLogin) {
      return (
        <LoginView
          onLoginSuccess={handleLoginSuccess}
          onSwitchToAccounts={() => {
            setShowLogin(false);
            setSelectedView('accounts');
          }}
        />
      );
    }

    switch (selectedView) {
      case 'home':
        return (
          <HomeView
            selectedVersion={selectedVersion}
            onVersionSelect={setSelectedVersion}
            currentAccount={currentAccount}
            onOpenSettings={handleOpenSettings}
          />
        );
      case 'versions':
        return <VersionManager onSelectVersion={setSelectedVersion} />;
      case 'accounts':
        return (
          <AccountManager
            onSelectAccount={handleSelectAccount}
            onAddAccount={handleAddAccount}
          />
        );
      case 'settings':
        return <SettingsView />;
      default:
        return (
          <HomeView
            selectedVersion={selectedVersion}
            onVersionSelect={setSelectedVersion}
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

  return (
    <TooltipProvider>
      <div className="flex h-screen overflow-hidden bg-base text-ink">
        {/* Persistent sidebar */}
        <aside className="flex w-[248px] shrink-0 flex-col border-r border-edge bg-panel">
          <div className="px-6 pb-8 pt-8">
            <div className="flex items-center gap-3">
              <span className="flex size-8 items-center justify-center rounded-md border border-edge-strong bg-white/6 text-sm font-bold text-ink">
                S
              </span>
              <span className="text-base font-semibold text-ink">Samuel Client</span>
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
                  <item.icon className="size-5 shrink-0" aria-hidden />
                  {t(item.labelKey)}
                </button>
              );
            })}
          </nav>

          <div className="mt-auto p-4">
            {currentAccount ? (
              <button
                type="button"
                onClick={() => {
                  setSelectedView('accounts');
                  setShowLogin(false);
                }}
                className="flex w-full items-center gap-3 rounded-md border border-edge bg-raised p-3 text-left transition-colors duration-150 hover:border-edge-strong"
              >
                <span className="flex size-10 shrink-0 items-center justify-center rounded-full border border-edge-strong bg-white/6 text-sm font-semibold text-ink">
                  {currentAccount.username.charAt(0).toUpperCase()}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-base font-medium text-ink">
                    {currentAccount.username}
                  </span>
                  <Badge
                    tone={currentAccount.authType === 'hrpauth' ? 'success' : 'neutral'}
                    className="mt-1"
                  >
                    {currentAccount.authType === 'hrpauth' ? t('play.hrpauth') : t('play.offline')}
                  </Badge>
                </span>
              </button>
            ) : (
              <Button variant="secondary" className="w-full" onClick={() => setShowLogin(true)}>
                {t('nav.signIn')}
              </Button>
            )}
          </div>
        </aside>

        {/* Single-column main content */}
        <main className="flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[980px] px-14 py-12">
            <header className="mb-10">
              <h1 className="text-display font-semibold tracking-tight text-ink">{t(getCurrentTitle())}</h1>
            </header>
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
    </TooltipProvider>
  );
}
