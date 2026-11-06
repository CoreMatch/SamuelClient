import { useState, useEffect } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { Asset } from '@ha/ui';
import { StoredAccount, ACCOUNTS_STORAGE_KEY, migrateAccountsIfNeeded } from '../accountStorage';
import { EmptyState } from './states';
import { t } from '../i18n';
import { Button, IconButton } from '../ui/button';
import { Badge } from '../ui/badge';
import { AlertDialog } from '../ui/dialog';
import { Callout } from '../ui/callout';

interface AccountManagerProps {
  onSelectAccount: (account: StoredAccount) => void;
  onAddAccount: () => void;
}

export default function AccountManager({ onSelectAccount, onAddAccount }: AccountManagerProps) {
  const [accounts, setAccounts] = useState<StoredAccount[]>([]);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [accountToDelete, setAccountToDelete] = useState<StoredAccount | null>(null);
  const [legacyNotice, setLegacyNotice] = useState('');

  useEffect(() => {
    loadAccounts();
  }, []);

  const loadAccounts = () => {
    try {
      migrateAccountsIfNeeded();
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
    // Legacy (non-HRPAuth Yggdrasil) accounts cannot be used anymore; give
    // a short readable notice instead of selecting or silently dropping them.
    if (account.legacy) {
      setLegacyNotice(t('accounts.legacyNotice'));
      return;
    }
    setLegacyNotice('');
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

  return (
    <div>
      <div className="mb-3 flex justify-end">
        <Button onClick={handleAddAccount}>
          <Plus className="size-5" aria-hidden />
          {t('accounts.add')}
        </Button>
      </div>

      {legacyNotice && (
        <div className="mb-4">
          <Callout tone="warning" onDismiss={() => setLegacyNotice('')}>
            {legacyNotice}
          </Callout>
        </div>
      )}

      {accounts.length === 0 ? (
        <EmptyState
          asset="state-not-logged-in"
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
              <Asset id="mode-avatar" size={40} className="text-ink-faint" />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="truncate text-base font-medium text-ink">{account.username}</span>
                  {account.lastUsed && (
                    <Asset id="icon-check" size={20} className="text-ok" title={t('accounts.lastUsed')} />
                  )}
                </div>
                <div className="mt-1.5 flex items-center gap-2">
                  {account.legacy ? (
                    <Badge tone="warning">{t('accounts.legacy')}</Badge>
                  ) : (
                    <Badge tone={account.authType === 'hrpauth' ? 'success' : 'neutral'}>
                      {account.authType === 'hrpauth' ? t('accounts.hrpauth') : t('accounts.offline')}
                    </Badge>
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
