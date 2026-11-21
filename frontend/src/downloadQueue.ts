export type DownloadTaskStatus = 'active' | 'completed' | 'failed' | 'cancelled';

export interface DownloadTask {
  versionId: string;
  name: string;
  stage: string;
  status: DownloadTaskStatus;
  downloaded: number;
  total: number;
  percent: number | null;
  error: string;
  updatedAt: number;
}

export interface DownloadProgressEvent {
  version_id?: string;
  stage?: string;
  name?: string;
  downloaded?: number;
  total?: number;
  error?: string;
}

const MAX_FINISHED_TASKS = 5;

export function updateDownloadQueue(
  tasks: DownloadTask[],
  event: DownloadProgressEvent,
  now = Date.now(),
): DownloadTask[] {
  if (!event.version_id || !event.stage) return tasks;

  const previous = tasks.find(task => task.versionId === event.version_id);
  const status: DownloadTaskStatus = event.stage === 'completed'
    ? 'completed'
    : event.stage === 'failed'
      ? 'failed'
      : event.stage === 'cancelled'
        ? 'cancelled'
        : 'active';
  const downloaded = typeof event.downloaded === 'number'
    ? event.downloaded
    : previous?.downloaded ?? 0;
  const total = typeof event.total === 'number'
    ? event.total
    : previous?.total ?? 0;
  const percent = status === 'completed'
    ? 100
    : status === 'active' && total > 0
      ? Math.min(100, Math.floor((downloaded / total) * 100))
      : status === 'active'
        ? null
        : previous?.percent ?? null;
  const nextTask: DownloadTask = {
    versionId: event.version_id,
    name: event.name || previous?.name || event.version_id,
    stage: event.stage,
    status,
    downloaded,
    total,
    percent,
    error: event.error || '',
    updatedAt: now,
  };
  const next = [...tasks.filter(task => task.versionId !== event.version_id), nextTask];
  const active = next.filter(task => task.status === 'active');
  const finished = next
    .filter(task => task.status !== 'active')
    .sort((a, b) => b.updatedAt - a.updatedAt)
    .slice(0, MAX_FINISHED_TASKS);

  return [...active, ...finished].sort((a, b) => {
    if (a.status === 'active' && b.status !== 'active') return -1;
    if (a.status !== 'active' && b.status === 'active') return 1;
    return b.updatedAt - a.updatedAt;
  });
}
