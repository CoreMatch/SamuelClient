import type { ReactNode } from 'react';
import { CircleAlert } from 'lucide-react';
import { t } from '../i18n';
import { Button } from '../ui/button';
import { Skeleton } from '../ui/skeleton';

// Unified empty/loading/error states (competitive-ui-research 04 §A5):
// empty = "nothing here yet + next action", loading = skeleton (no bare
// spinner jumping), error = one readable reason + Retry.

interface LoadingSkeletonProps {
  rows?: number;
}

export function LoadingSkeleton({ rows = 3 }: LoadingSkeletonProps) {
  return <Skeleton rows={rows} />;
}

interface ErrorStateProps {
  // What failed to load, inserted into the readable message (i18n).
  data: string;
  onRetry: () => void;
}

export function ErrorState({ data, onRetry }: ErrorStateProps) {
  return (
    <div className="rounded-lg border border-edge bg-panel px-6 py-10 text-center">
      <CircleAlert className="mx-auto size-10 text-danger" aria-hidden />
      <p className="mx-auto mt-3 max-w-md text-sm leading-relaxed text-ink">
        {t('common.loadError', { data })}
      </p>
      <div className="mt-4 flex justify-center">
        <Button variant="secondary" onClick={onRetry}>
          {t('common.retry')}
        </Button>
      </div>
    </div>
  );
}

interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  hint?: string;
  actionLabel?: string;
  onAction?: () => void;
}

export function EmptyState({ icon, title, hint, actionLabel, onAction }: EmptyStateProps) {
  return (
    <div className="rounded-lg border border-edge bg-panel px-6 py-12 text-center">
      {icon && <div className="flex justify-center text-ink-faint">{icon}</div>}
      <h3 className="mt-3 text-base font-semibold text-ink">{title}</h3>
      {hint && <p className="mx-auto mt-1.5 max-w-sm text-sm leading-relaxed text-ink-muted">{hint}</p>}
      {actionLabel && onAction && (
        <div className="mt-5 flex justify-center">
          <Button variant="secondary" onClick={onAction}>
            {actionLabel}
          </Button>
        </div>
      )}
    </div>
  );
}
