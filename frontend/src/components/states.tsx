import type { ReactNode } from 'react';
import { CircleAlert } from 'lucide-react';
import { t } from '../i18n';
import { Button } from '../ui/button';
import { Skeleton } from '../ui/skeleton';

// Unified empty/loading/error states: one large glyph (illustration slot —
// swapped for the ha-assets state art once @ha/ui wraps it), one short line,
// one action. Full explanations stay out of the main visual.

interface LoadingSkeletonProps {
  rows?: number;
}

export function LoadingSkeleton({ rows = 3 }: LoadingSkeletonProps) {
  return <Skeleton rows={rows} />;
}

interface ErrorStateProps {
  // What failed to load, inserted into the short message (i18n).
  data: string;
  onRetry: () => void;
}

export function ErrorState({ data, onRetry }: ErrorStateProps) {
  return (
    <div className="rounded-lg bg-panel px-8 py-14 text-center">
      <CircleAlert className="mx-auto size-14 text-danger/80" aria-hidden />
      <p className="mx-auto mt-5 max-w-md text-base text-ink">
        {t('common.loadError', { data })}
      </p>
      <div className="mt-6 flex justify-center">
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
  actionLabel?: string;
  onAction?: () => void;
}

export function EmptyState({ icon, title, actionLabel, onAction }: EmptyStateProps) {
  return (
    <div className="rounded-lg bg-panel px-8 py-16 text-center">
      {icon && <div className="flex justify-center text-ink-faint">{icon}</div>}
      <h3 className="mt-5 text-xl font-semibold text-ink">{title}</h3>
      {actionLabel && onAction && (
        <div className="mt-6 flex justify-center">
          <Button variant="secondary" onClick={onAction}>
            {actionLabel}
          </Button>
        </div>
      )}
    </div>
  );
}
