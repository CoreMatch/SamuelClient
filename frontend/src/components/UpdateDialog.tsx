import { Quit } from '../../wailsjs/runtime/runtime';
import { t } from '../i18n';
import type { UpdateStatus } from '../useUpdateStatus';
import { Button } from '../ui/button';
import { Dialog } from '../ui/dialog';
import { Progress } from '../ui/progress';

interface UpdateDialogProps {
  status: UpdateStatus;
  open: boolean;
  onInstall: () => void;
  onDismiss: () => void;
  onRetry: () => void;
}

export function UpdateDialog({ status, open, onInstall, onDismiss, onRetry }: UpdateDialogProps) {
  const busy = status.state === 'downloading' || status.state === 'installing' || status.state === 'ready';
  const dismissible = !status.mandatory && !busy;
  const progress = status.total > 0 ? Math.min(100, Math.round(status.downloaded / status.total * 100)) : null;
  const title = status.state === 'downloading'
    ? t('updater.downloading')
    : status.state === 'installing' || status.state === 'ready'
      ? t('updater.installing')
      : status.state === 'error'
        ? t('updater.failed')
        : t('updater.available');

  return (
    <Dialog
      open={open}
      onOpenChange={next => { if (next || dismissible) onDismiss(); }}
      title={title}
      closeLabel={t('updater.dismiss')}
      dismissible={dismissible}
    >
      <div className="space-y-5">
        <div className="flex items-center justify-between gap-4">
          <span className="text-sm text-ink-muted">{t('updater.version', { version: status.version })}</span>
          {status.mandatory && (
            <span className="rounded-full bg-danger/10 px-3 py-1 text-xs font-semibold text-danger">
              {t('updater.required')}
            </span>
          )}
        </div>
        {status.allowDowngrade && (
          <p className="text-sm font-medium text-warning">{t('updater.rollback')}</p>
        )}
        {status.notes && (
          <p className="whitespace-pre-wrap text-sm leading-relaxed text-ink-muted">{status.notes}</p>
        )}
        {status.state === 'downloading' && (
          <div className="space-y-2">
            <Progress value={progress} />
            <p className="text-right text-xs text-ink-faint">
              {progress === null ? t('updater.preparing') : t('updater.progress', { progress })}
            </p>
          </div>
        )}
        {status.state === 'error' && status.error && (
          <p role="alert" className="rounded-md bg-danger/10 p-3 text-sm text-danger">{status.error}</p>
        )}
        <div className="flex flex-wrap justify-end gap-3">
          {status.mandatory && !busy && (
            <Button variant="ghost" onClick={() => Quit()}>{t('updater.exit')}</Button>
          )}
          {dismissible && status.state !== 'error' && (
            <Button variant="secondary" onClick={onDismiss}>{t('updater.later')}</Button>
          )}
          {status.state === 'error' ? (
            <Button onClick={onRetry}>{t('updater.retry')}</Button>
          ) : status.state === 'available' ? (
            <Button onClick={onInstall}>{t('updater.installNow')}</Button>
          ) : (
            <Button loading disabled>{t('updater.pleaseWait')}</Button>
          )}
        </div>
      </div>
    </Dialog>
  );
}
