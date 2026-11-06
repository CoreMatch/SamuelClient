import { useState, useEffect } from 'react';
import { BadgeCheck, Plus, Trash2, Users } from 'lucide-react';
import { StoredAccount, ACCOUNTS_STORAGE_KEY } from '../accountStorage';
import { EmptyState } from './states';
import { t } from '../i18n';
import { Button, IconButton } from '../ui/button';
import { Badge } from '../ui/badge';
import { AlertDialog } from '../ui/dialog';

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

  const authServerHost = (account: StoredAccount): string => {
    if (!account.authServer) return '';
    try {
      return new URL(account.authServer).hostname;
    } catch {
      return account.authServer;
    }
  };

  return (
    <div>
      <div className="mb-3 flex justify-end">
        <Button onClick={handleAddAccount}>
          <Plus className="size-5" aria-hidden />
          {t('accounts.add')}
        </Button>
      </div>

      {accounts.length === 0 ? (
        <EmptyState
          icon={<Users className="size-14 opacity-60" aria-hidden />}
          title={t('accounts.emptyTitle')}
          actionLabel={t('accounts.add')}
          onAction={handleAddAccount}
        />
      ) : (
        <div className="rounded-lg bg-panel">
          {accounts.map((account, index) => (
            <div
              key={account.id}
              role="button"
              tabIndex={0}
              onClick={() => handleSelectAccount(account)}
              onKeyDown={e => {
                if (e.key === 'Enter' || e.key === ' ') {
                  e.preventDefault();
                  handleSelectAccount(account);
                }
              }}
              aria-label={t('accounts.select', { username: account.username })}
              className={`flex w-full cursor-pointer items-center gap-4 px-5 py-5 text-left transition-colors duration-150 hover:bg-white/4 ${
                index > 0 ? 'border-t border-edge' : ''
              } ${account.lastUsed ? 'bg-white/4' : ''}`}
            >
              <span className="flex size-11 shrink-0 items-center justify-center rounded-full border border-edge-strong bg-white/6 text-base font-semibold text-ink">
                {account.username.charAt(0).toUpperCase()}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate text-base font-medium text-ink">{account.username}</span>
                  {account.lastUsed && (
                    <BadgeCheck className="size-5 shrink-0 text-ok" aria-label={t('accounts.lastUsed')} />
                  )}
                </div>
                <div className="mt-1.5 flex items-center gap-2">
                  <Badge tone={account.authType === 'authlib-injector' ? 'success' : 'neutral'}>
                    {account.authType === 'authlib-injector' ? t('accounts.online') : t('accounts.offline')}
                  </Badge>
                  {account.authServer && (
                    <span className="truncate text-sm text-ink-faint">{authServerHost(account)}</span>
                  )}
                </div>
              </div>
              <IconButton
                label={t('common.delete')}
                tone="danger"
                onClick={e => {
                  e.stopPropagation();
                  handleDeleteClick(account);
                }}
              >
                <Trash2 className="size-5" />
              </IconButton>
            </div>
          ))}
        </div>
      )}

      <AlertDialog
        open={deleteDialogOpen}
        onOpenChange={open => {
          setDeleteDialogOpen(open);
          if (!open) setAccountToDelete(null);
        }}
        title={t('accounts.deleteTitle')}
        description={t('accounts.deleteConfirm', { username: accountToDelete?.username ?? '' })}
        confirmLabel={t('common.delete')}
        cancelLabel={t('common.cancel')}
        onConfirm={handleConfirmDelete}
      />
    </div>
  );
}
