import { useState, useEffect } from 'react';
import { ThemeProvider } from '@mui/material/styles';
import CssBaseline from '@mui/material/CssBaseline';
import { Box, Drawer, List, ListItem, ListItemButton, ListItemIcon, ListItemText, AppBar, Toolbar, Typography, IconButton } from '@mui/material';
import { Gamepad, Home, Settings, Group, Layers, Menu, ArrowBack } from '@mui/icons-material';
import HomeView from './components/HomeView';
import VersionManager from './components/VersionManager';
import AccountManager from './components/AccountManager';
import ModManager from './components/ModManager';
import SettingsView from './components/SettingsView';
import HandoffBanner from './components/HandoffBanner';
import LoginView, { LoggedInAccount } from './components/LoginView';
import { StoredAccount, saveAccount, restoreLastUsedAccount } from './accountStorage';
import { InstalledVersion, HandoffPayload } from './types';
import { t } from './i18n';
import { darkTheme, tokens } from './theme';
import {
  GetPendingHandoff,
  DismissHandoff,
  GetInstalledVersions,
  LaunchGame,
} from '../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../wailsjs/runtime/runtime';

const drawerWidth = 240;

interface NavItem {
  id: string;
  label: string;
  icon: typeof Home;
}

const navItems: NavItem[] = [
  { id: 'home', label: 'Play', icon: Home },
  { id: 'versions', label: 'Versions', icon: Gamepad },
  { id: 'accounts', label: 'Accounts', icon: Group },
  { id: 'mods', label: 'Mods', icon: Layers },
  { id: 'settings', label: 'Settings', icon: Settings },
];

export default function App() {
  const [selectedView, setSelectedView] = useState<string>('home');
  const [selectedVersion, setSelectedVersion] = useState<InstalledVersion | null>(null);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [currentAccount, setCurrentAccount] = useState<StoredAccount | null>(null);
  const [showLogin, setShowLogin] = useState(false);

  // Match handoff state (INTEROP-SPEC): pending payload, readable
  // degradation notice, join progress and failure text.
  const [handoff, setHandoff] = useState<HandoffPayload | null>(null);
  const [handoffNotice, setHandoffNotice] = useState('');
  const [joiningMatch, setJoiningMatch] = useState(false);
  const [joinError, setJoinError] = useState('');

  useEffect(() => {
    // Restore the last-used account. authlib-injector tokens are validated
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
        auth_server: account.authServer || '',
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

  const handleOpenModsFolder = () => {
    // Open mods folder in file explorer
    const modsPath = getModsFolderPath();
    window.open(`file://${modsPath}`, '_blank');
  };

  const getModsFolderPath = () => {
    const home = '/home/lnb/.minecraft';
    return `${home}/mods`;
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
      case 'mods':
        return <ModManager onOpenModsFolder={handleOpenModsFolder} />;
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

  const handleDrawerToggle = () => {
    setMobileOpen(!mobileOpen);
  };

  const getCurrentTitle = () => {
    if (showLogin) return 'Login';
    const item = navItems.find(n => n.id === selectedView);
    return item?.label || 'Play';
  };

  return (
    <ThemeProvider theme={darkTheme}>
      <CssBaseline />
      <Box sx={{ display: 'flex', minHeight: '100vh', backgroundColor: 'background.default' }}>
        <AppBar
          position="fixed"
          sx={{
            width: { sm: `calc(100% - ${drawerWidth}px)` },
            ml: { sm: `${drawerWidth}px` },
            backgroundColor: tokens.color.deep,
          }}
        >
        <Toolbar>
          <IconButton
            color="inherit"
            aria-label="open drawer"
            edge="start"
            onClick={handleDrawerToggle}
            sx={{ mr: 2, display: { sm: 'none' } }}
          >
            <Menu />
          </IconButton>
          <Typography variant="h6" noWrap component="div" sx={{ flexGrow: 1 }}>
            {getCurrentTitle()}
          </Typography>
          {!showLogin && !currentAccount && (
            <IconButton color="inherit" onClick={() => setShowLogin(true)}>
              <ArrowBack />
            </IconButton>
          )}
        </Toolbar>
      </AppBar>

      <Box
        component="nav"
        sx={{ width: { sm: drawerWidth }, flexShrink: { sm: 0 } }}
        aria-label="mailbox folders"
      >
        <Drawer
          variant="temporary"
          open={mobileOpen}
          onClose={handleDrawerToggle}
          ModalProps={{
            keepMounted: true,
          }}
          sx={{
            display: { xs: 'block', sm: 'none' },
            '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth },
          }}
        >
          <Box onClick={handleDrawerToggle} sx={{ textAlign: 'center' }}>
            <Typography variant="h6" sx={{ my: 2, color: 'white' }}>Minecraft</Typography>
            <List>
              {navItems.map((item) => (
                <ListItem key={item.id}>
                  <ListItemButton
                    selected={selectedView === item.id && !showLogin}
                    onClick={() => {
                      setSelectedView(item.id);
                      setShowLogin(false);
                    }}
                  >
                    <ListItemIcon sx={{ color: selectedView === item.id && !showLogin ? 'white' : tokens.color.textMuted }}>
                      <item.icon />
                    </ListItemIcon>
                    <ListItemText primary={item.label} sx={{ color: selectedView === item.id && !showLogin ? 'white' : tokens.color.text }} />
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          </Box>
        </Drawer>

        <Drawer
          variant="permanent"
          sx={{
            display: { xs: 'none', sm: 'block' },
            '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth, backgroundColor: tokens.color.deep },
          }}
          open
        >
          <Box sx={{ textAlign: 'center', py: 4 }}>
            <Typography variant="h6" sx={{ color: 'white' }}>Samuel Client</Typography>
          </Box>
          <List>
            {navItems.map((item) => (
              <ListItem key={item.id}>
                <ListItemButton
                  selected={selectedView === item.id && !showLogin}
                  onClick={() => {
                    setSelectedView(item.id);
                    setShowLogin(false);
                  }}
                  sx={{
                    borderRadius: tokens.radius.md,
                    mx: 1,
                    mb: 0.5,
                    '&.Mui-selected': { backgroundColor: tokens.color.accent, '&:hover': { backgroundColor: tokens.color.accentDark } },
                  }}
                >
                  <ListItemIcon sx={{ color: selectedView === item.id && !showLogin ? 'white' : tokens.color.textMuted }}>
                    <item.icon />
                  </ListItemIcon>
                  <ListItemText primary={item.label} sx={{ color: selectedView === item.id && !showLogin ? 'white' : tokens.color.text }} />
                </ListItemButton>
              </ListItem>
            ))}
          </List>
        </Drawer>
      </Box>

      <Box
        component="main"
        sx={{
          flexGrow: 1,
          p: 3,
          width: { sm: `calc(100% - ${drawerWidth}px)` },
          backgroundColor: 'background.default',
          minHeight: '100vh',
        }}
      >
        <Toolbar />
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
      </Box>
      </Box>
    </ThemeProvider>
  );
}
