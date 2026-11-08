import { useState, useEffect } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { Asset } from '@ha/ui';
import {
  StoredAccount,
  ACCOUNTS_STORAGE_KEY,
  migrateAccountsIfNeeded,
  persistAccountsMetadata,
  loadAccountTokens,
  deleteAccountTokens,
} from '../accountStorage';
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
  const [storageWarning, setStorageWarning] = useState('');

  useEffect(() => {
    loadAccounts();
  }, []);

  const loadAccounts = async () => {
    try {
      const migration = await migrateAccountsIfNeeded();
      if (!migration.done) {
        // Tokens could not move to the keychain; the old records are intact
        // and nothing is lost — surface the reason instead of failing quiet.
        setStorageWarning(migration.keychainError);
      }
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
      persistAccountsMetadata(newAccounts);
      setAccounts(newAccounts);
    } catch (err) {
      console.error('Failed to save accounts:', err);
    }
  };

  const handleSelectAccount = async (account: StoredAccount) => {
    // Legacy (non-HRPAuth Yggdrasil) accounts cannot be used anymore; give
    // a short readable notice instead of selecting or silently dropping them.
    if (account.legacy) {
      setLegacyNotice(t('accounts.legacyNotice'));
      return;
    }
    setLegacyNotice('');
    // Tokens live in the OS keychain (ADR-0008); hydrate them before handing
    // the account to the launch flow. A miss means the session is gone — ask
    // for a fresh sign-in instead of selecting a dead account.
    const tokens = await loadAccountTokens(account.id);
    if (!tokens) {
      setLegacyNotice(t('accounts.reLoginNeeded'));
      return;
    }
    // Update last used
    const updatedAccounts = accounts.map(acc => ({
      ...acc,
      lastUsed: acc.id === account.id,
    }));
    saveAccounts(updatedAccounts);
    onSelectAccount({ ...account, ...tokens });
  };

  const handleDeleteClick = (account: StoredAccount) => {
    setAccountToDelete(account);
    setDeleteDialogOpen(true);
  };

  const handleConfirmDelete = async () => {
    if (accountToDelete) {
      // Drop the keychain entry as well as the metadata; best-effort by
      // design (deleteAccountTokens never throws).
      await deleteAccountTokens(accountToDelete.id);
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

      {storageWarning && (
        <div className="mb-4">
          <Callout tone="danger" onDismiss={() => setStorageWarning('')}>
            {t('app.keychainUnavailable', { error: storageWarning })}
          </Callout>
        </div>
      )}

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
                    <Badge tone="success">{t('accounts.hrpauth')}</Badge>
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
