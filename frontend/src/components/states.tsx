import { Asset, type AssetId } from '@ha/ui';
import { t } from '../i18n';
import { Button } from '../ui/button';
import { Skeleton } from '../ui/skeleton';

// Unified empty/loading/error states: one ha-assets illustration, one short
// line, one action. Full explanations stay out of the main visual.

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
      <Asset id="state-network-failure" size={200} className="mx-auto" />
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
  asset?: AssetId;
  title: string;
  actionLabel?: string;
  onAction?: () => void;
}

export function EmptyState({ asset, title, actionLabel, onAction }: EmptyStateProps) {
  return (
    <div className="rounded-lg bg-panel px-8 py-16 text-center">
      {asset && <Asset id={asset} size={200} className="mx-auto" />}
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
