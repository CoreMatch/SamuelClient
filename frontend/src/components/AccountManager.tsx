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
import { EmptyState } from './states';
import { t } from '../i18n';
import { tokens } from '../theme';

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
      <Wifi sx={{ color: tokens.color.success }} />
    ) : (
      <WifiOff sx={{ color: 'text.secondary' }} />
    );
  };

  const getAuthTypeLabel = (authType: string) => {
    return authType === 'authlib-injector' ? 'Online' : 'Offline';
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Button variant="contained" startIcon={<Add />} onClick={handleAddAccount}>
          Add Account
        </Button>
      </Box>

      <Typography variant="body2" sx={{ color: 'text.secondary', mb: 2 }}>
        Select an account to login. Click the delete button to remove an account.
      </Typography>

      {accounts.length === 0 ? (
        <EmptyState
          icon={<Person sx={{ fontSize: 64, mb: 2, opacity: 0.5, color: 'text.secondary' }} />}
          title={t('accounts.emptyTitle')}
          hint={t('accounts.emptyHint')}
          actionLabel="Add Account"
          onAction={handleAddAccount}
        />
      ) : (
        <List sx={{ backgroundColor: 'background.paper', borderRadius: tokens.radius.lg }}>
          {accounts.map((account, index) => (
            <Box key={account.id}>
              <ListItem
                sx={{
                  cursor: 'pointer',
                  '&:hover': { backgroundColor: 'rgba(255, 255, 255, 0.06)' },
                  backgroundColor: account.lastUsed ? 'rgba(255, 255, 255, 0.06)' : 'transparent',
                }}
                onClick={() => handleSelectAccount(account)}
              >
                <ListItemAvatar>
                  <Avatar sx={{ backgroundColor: tokens.color.accent }}>
                    <Person />
                  </Avatar>
                </ListItemAvatar>
                <ListItemText
                  primary={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <Typography sx={{ color: 'text.primary' }}>{account.username}</Typography>
                      {account.lastUsed && (
                        <CheckCircle sx={{ fontSize: 16, color: tokens.color.success }} />
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
                          backgroundColor: 'rgba(255, 255, 255, 0.08)',
                          color: account.authType === 'authlib-injector' ? tokens.color.success : 'text.secondary',
                        }}
                      />
                      {account.authServer && (
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
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
                    sx={{ color: tokens.color.danger }}
                  >
                    <Delete />
                  </IconButton>
                </ListItemSecondaryAction>
              </ListItem>
              {index < accounts.length - 1 && <Divider sx={{ borderColor: tokens.color.divider }} />}
            </Box>
          ))}
        </List>
      )}

      <Dialog
        open={deleteDialogOpen}
        onClose={() => setDeleteDialogOpen(false)}
        slotProps={{ paper: { sx: { backgroundColor: 'background.paper', backgroundImage: 'none' } } }}
      >
        <DialogTitle sx={{ color: 'text.primary' }}>Delete Account</DialogTitle>
        <DialogContent>
          <Typography sx={{ color: 'text.secondary' }}>
            Are you sure you want to delete the account "{accountToDelete?.username}"?
            This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ p: 2 }}>
          <Button onClick={() => setDeleteDialogOpen(false)} sx={{ color: 'text.secondary' }}>
            Cancel
          </Button>
          <Button
            onClick={handleConfirmDelete}
            sx={{ backgroundColor: tokens.color.danger, color: tokens.color.deep, '&:hover': { backgroundColor: '#ef5350' } }}
          >
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
