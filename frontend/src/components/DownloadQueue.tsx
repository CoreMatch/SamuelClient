import { Download, X } from 'lucide-react';
import { Badge } from '../ui/badge';
import { IconButton } from '../ui/button';
import { Progress } from '../ui/progress';
import { t } from '../i18n';
import type { DownloadTask } from '../downloadQueue';
import type { I18nKey } from '../i18n';

interface DownloadQueueProps {
  tasks: DownloadTask[];
  onCancel: (versionId: string) => void;
  onDismiss: (versionId: string) => void;
}

function taskStageLabel(stage: string): I18nKey {
  switch (stage) {
    case 'jar':
      return 'install.step.jar';
    case 'library':
      return 'install.step.library';
    case 'native':
      return 'install.step.native';
    case 'asset':
      return 'install.step.asset';
    case 'loader':
      return 'install.step.loader';
    case 'java-download':
      return 'queue.stage.javaDownload';
    case 'java-extract':
      return 'queue.stage.javaExtract';
    case 'pack-archive':
      return 'packs.stage.archive';
    case 'pack-files':
      return 'packs.stage.files';
    case 'pack-overrides':
      return 'packs.stage.overrides';
    default:
      return 'queue.stage.working';
  }
}

function taskStatusLabel(task: DownloadTask): I18nKey {
  switch (task.status) {
    case 'completed':
      return 'queue.completed';
    case 'failed':
      return 'queue.failed';
    case 'cancelled':
      return 'queue.cancelled';
    default:
      return 'queue.active';
  }
}

export default function DownloadQueue({ tasks, onCancel, onDismiss }: DownloadQueueProps) {
  if (tasks.length === 0) return null;

  const activeCount = tasks.filter(task => task.status === 'active').length;

  return (
    <section
      aria-label={t('queue.title')}
      className="mx-4 mt-5 flex min-h-0 flex-1 flex-col border-t border-edge px-1 pt-4"
    >
      <header className="flex items-center gap-2 px-1">
        <Download className="size-4 shrink-0 text-accent" aria-hidden />
        <h2 className="min-w-0 flex-1 truncate text-sm font-semibold text-ink">{t('queue.title')}</h2>
        <Badge tone={activeCount > 0 ? 'info' : 'neutral'} className="shrink-0 px-2 py-0.5 text-xs">
          {activeCount}
        </Badge>
      </header>

      <div className="mt-3 min-h-0 flex-1 space-y-2 overflow-y-auto pb-2">
        {tasks.map(task => (
          <article
            key={task.versionId}
            aria-label={`${task.versionId}: ${t(taskStatusLabel(task))}`}
            className="rounded-md border border-edge bg-raised px-2.5 py-2"
          >
            <div className="flex items-start gap-1">
              <span className="min-w-0 flex-1 truncate text-xs font-medium text-ink" title={task.versionId}>
                {task.versionId.startsWith('modpack-') ? t('packs.tab') : task.versionId}
              </span>
              {task.status === 'active' ? (
                <IconButton
                  label={t('queue.cancelTask', { task: task.versionId })}
                  className="size-6 shrink-0"
                  onClick={() => onCancel(task.versionId)}
                >
                  <X className="size-3.5" aria-hidden />
                </IconButton>
              ) : (
                <IconButton
                  label={t('queue.dismissTask', { task: task.versionId })}
                  className="size-6 shrink-0"
                  onClick={() => onDismiss(task.versionId)}
                >
                  <X className="size-3.5" aria-hidden />
                </IconButton>
              )}
            </div>
            {task.name && task.name !== task.versionId && (
              <p className="mt-0.5 truncate text-[10px] text-ink-faint" title={task.name}>{task.name}</p>
            )}
            <p className={`mt-1 truncate text-[10px] ${
              task.status === 'failed' ? 'text-danger' : 'text-ink-muted'
            }`}>
              {task.status === 'active'
                ? t(taskStageLabel(task.stage))
                : task.status === 'failed' && task.error
                  ? task.error
                  : t(taskStatusLabel(task))}
            </p>
            {task.status === 'active' && (
              <div className="mt-2" aria-label={t('queue.progressFor', { task: task.versionId })}>
                <Progress value={task.percent} />
              </div>
            )}
            {task.status === 'active' && task.percent != null && (
              <p className="mt-1 text-right font-mono text-[10px] text-ink-faint">{task.percent}%</p>
            )}
          </article>
        ))}
      </div>
    </section>
  );
}
