import { useState, useEffect } from 'react';
import { Box, Drawer, List, ListItem, ListItemButton, ListItemIcon, ListItemText, AppBar, Toolbar, Typography, IconButton } from '@mui/material';
import { Gamepad, Home, Settings, Group, Layers, Menu, ArrowBack } from '@mui/icons-material';
import HomeView from './components/HomeView';
import VersionManager from './components/VersionManager';
import AccountManager, { StoredAccount, saveAccount } from './components/AccountManager';
import ModManager from './components/ModManager';
import SettingsView from './components/SettingsView';
import LoginView, { LoggedInAccount } from './components/LoginView';

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
  const [selectedVersion, setSelectedVersion] = useState<any>(null);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [currentAccount, setCurrentAccount] = useState<StoredAccount | null>(null);
  const [showLogin, setShowLogin] = useState(false);

  useEffect(() => {
    // Load last used account
    loadLastUsedAccount();
  }, []);

  const loadLastUsedAccount = () => {
    try {
      const stored = localStorage.getItem('minecraft_accounts');
      if (stored) {
        const accounts: StoredAccount[] = JSON.parse(stored);
        const lastUsed = accounts.find(acc => acc.lastUsed);
        if (lastUsed) {
          setCurrentAccount(lastUsed);
        }
      }
    } catch (err) {
      console.error('Failed to load last used account:', err);
    }
  };

  const handleLoginSuccess = (account: LoggedInAccount) => {
    const savedAccount = saveAccount(account);
    setCurrentAccount(savedAccount);
    setShowLogin(false);
    setSelectedView('home');
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
    <Box sx={{ display: 'flex' }}>
      <AppBar
        position="fixed"
        sx={{
          width: { sm: `calc(100% - ${drawerWidth}px)` },
          ml: { sm: `${drawerWidth}px` },
          backgroundColor: '#282c34',
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
                    <ListItemIcon sx={{ color: selectedView === item.id && !showLogin ? 'white' : '#9ca3af' }}>
                      <item.icon />
                    </ListItemIcon>
                    <ListItemText primary={item.label} sx={{ color: selectedView === item.id && !showLogin ? 'white' : '#d1d5db' }} />
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
            '& .MuiDrawer-paper': { boxSizing: 'border-box', width: drawerWidth, backgroundColor: '#212529' },
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
                  sx={{ '&.Mui-selected': { backgroundColor: '#4CAF50' } }}
                >
                  <ListItemIcon sx={{ color: selectedView === item.id && !showLogin ? 'white' : '#9ca3af' }}>
                    <item.icon />
                  </ListItemIcon>
                  <ListItemText primary={item.label} sx={{ color: selectedView === item.id && !showLogin ? 'white' : '#d1d5db' }} />
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
          backgroundColor: '#1a1d21',
          minHeight: '100vh',
        }}
      >
        <Toolbar />
        {renderView()}
      </Box>
    </Box>
  );
}
