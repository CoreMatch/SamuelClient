import { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  List,
  ListItem,
  ListItemAvatar,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Avatar,
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Chip,
  Paper,
  Divider,
} from '@mui/material';
import {
  Person,
  Delete,
  Add,
  CheckCircle,
  Wifi,
  WifiOff,
} from '@mui/icons-material';

import { StoredAccount, ACCOUNTS_STORAGE_KEY } from '../accountStorage';

interface AccountManagerProps {
  onSelectAccount: (account: StoredAccount) => void;
  onAddAccount: () => void;
}

export default function AccountManager({ onSelectAccount, onAddAccount }: AccountManagerProps) {
  const [accounts, setAccounts] = useState<StoredAccount[]>([]);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [accountToDelete, setAccountToDelete] = useState<StoredAccount | null>(null);

  useEffect(() => {
    loadAccounts();
  }, []);

  const loadAccounts = () => {
    try {
      const stored = localStorage.getItem(ACCOUNTS_STORAGE_KEY);
      if (stored) {
        const parsed = JSON.parse(stored);
        setAccounts(parsed);
      }
    } catch (err) {
      console.error('Failed to load accounts:', err);
    }
  };

  const saveAccounts = (newAccounts: StoredAccount[]) => {
    try {
      localStorage.setItem(ACCOUNTS_STORAGE_KEY, JSON.stringify(newAccounts));
      setAccounts(newAccounts);
    } catch (err) {
      console.error('Failed to save accounts:', err);
    }
  };

  const handleSelectAccount = (account: StoredAccount) => {
    // Update last used
    const updatedAccounts = accounts.map(acc => ({
      ...acc,
      lastUsed: acc.id === account.id,
    }));
    saveAccounts(updatedAccounts);
    onSelectAccount(account);
  };

  const handleDeleteClick = (account: StoredAccount) => {
    setAccountToDelete(account);
    setDeleteDialogOpen(true);
  };

  const handleConfirmDelete = () => {
    if (accountToDelete) {
      const newAccounts = accounts.filter(acc => acc.id !== accountToDelete.id);
      saveAccounts(newAccounts);
    }
    setDeleteDialogOpen(false);
    setAccountToDelete(null);
  };

  const handleAddAccount = () => {
    onAddAccount();
  };

  const getAuthTypeIcon = (authType: string) => {
    return authType === 'authlib-injector' ? (
      <Wifi sx={{ color: '#4CAF50' }} />
    ) : (
      <WifiOff sx={{ color: '#9ca3af' }} />
    );
  };

  const getAuthTypeLabel = (authType: string) => {
    return authType === 'authlib-injector' ? 'Online' : 'Offline';
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Button
          variant="contained"
          startIcon={<Add />}
          onClick={handleAddAccount}
          sx={{ backgroundColor: '#4CAF50' }}
        >
          Add Account
        </Button>
      </Box>

      <Typography variant="body2" sx={{ color: '#9ca3af', mb: 2 }}>
        Select an account to login. Click the delete button to remove an account.
      </Typography>

      {accounts.length === 0 ? (
        <Paper
          sx={{
            p: 4,
            textAlign: 'center',
            backgroundColor: '#2d3339',
            color: '#9ca3af',
          }}
        >
          <Person sx={{ fontSize: 64, mb: 2, opacity: 0.5 }} />
          <Typography variant="h6" sx={{ mb: 1 }}>
            No Accounts
          </Typography>
          <Typography variant="body2" sx={{ mb: 2 }}>
            Add an account to get started
          </Typography>
          <Button
            variant="outlined"
            startIcon={<Add />}
            onClick={handleAddAccount}
            sx={{ borderColor: '#4CAF50', color: '#4CAF50' }}
          >
            Add Account
          </Button>
        </Paper>
      ) : (
        <List sx={{ backgroundColor: '#282c34', borderRadius: 1 }}>
          {accounts.map((account, index) => (
            <Box key={account.id}>
              <ListItem
                sx={{
                  cursor: 'pointer',
                  '&:hover': { backgroundColor: '#323842' },
                  backgroundColor: account.lastUsed ? '#323842' : 'transparent',
                }}
                onClick={() => handleSelectAccount(account)}
              >
                <ListItemAvatar>
                  <Avatar sx={{ backgroundColor: '#4CAF50' }}>
                    <Person />
                  </Avatar>
                </ListItemAvatar>
                <ListItemText
                  primary={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Typography sx={{ color: 'white' }}>{account.username}</Typography>
                      {account.lastUsed && (
                        <CheckCircle sx={{ fontSize: 16, color: '#4CAF50' }} />
                      )}
                    </Box>
                  }
                  secondary={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
                      <Chip
                        size="small"
                        icon={getAuthTypeIcon(account.authType)}
                        label={getAuthTypeLabel(account.authType)}
                        sx={{
                          height: 24,
                          backgroundColor: account.authType === 'authlib-injector' ? '#2d3339' : '#374151',
                          color: account.authType === 'authlib-injector' ? '#4CAF50' : '#9ca3af',
                        }}
                      />
                      {account.authServer && (
                        <Typography variant="caption" sx={{ color: '#6b7280' }}>
                          {new URL(account.authServer).hostname}
                        </Typography>
                      )}
                    </Box>
                  }
                />
                <ListItemSecondaryAction>
                  <IconButton
                    edge="end"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDeleteClick(account);
                    }}
                    sx={{ color: '#ef4444' }}
                  >
                    <Delete />
                  </IconButton>
                </ListItemSecondaryAction>
              </ListItem>
              {index < accounts.length - 1 && <Divider sx={{ borderColor: '#374151' }} />}
            </Box>
          ))}
        </List>
      )}

      <Dialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
        slotProps={{ paper: { sx: { backgroundColor: '#282c34' } } }}
      >
        <DialogTitle sx={{ color: 'white' }}>Delete Account</DialogTitle>
        <DialogContent>
          <Typography sx={{ color: '#9ca3af' }}>
            Are you sure you want to delete the account "{accountToDelete?.username}"?
            This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ p: 2 }}>
          <Button onClick={() => setDeleteDialogOpen(false)} sx={{ color: '#9ca3af' }}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirmDelete}
            sx={{ backgroundColor: '#ef4444', color: 'white', '&:hover': { backgroundColor: '#dc2626' } }}
          >
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
