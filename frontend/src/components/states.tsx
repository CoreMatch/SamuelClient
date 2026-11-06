import { ReactNode } from 'react';
import { Box, Button, Skeleton, Typography } from '@mui/material';
import { ErrorOutlined } from '@mui/icons-material';
import { t } from '../i18n';
import { tokens } from '../theme';

// Unified empty/loading/error states (competitive-ui-research 04 §A5):
// empty = "nothing here yet + next action", loading = skeleton (no bare
// spinner jumping), error = one readable reason + Retry.

interface LoadingSkeletonProps {
  rows?: number;
}

export function LoadingSkeleton({ rows = 3 }: LoadingSkeletonProps) {
  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton
          key={i}
          variant="rounded"
          height={48}
          sx={{ borderRadius: tokens.radius.md, bgcolor: 'rgba(255, 255, 255, 0.08)' }}
        />
      ))}
    </Box>
  );
}

interface ErrorStateProps {
  // What failed to load, inserted into the readable message (i18n).
  data: string;
  onRetry: () => void;
}

export function ErrorState({ data, onRetry }: ErrorStateProps) {
  return (
    <Box
      sx={{
        p: 4,
        textAlign: 'center',
        backgroundColor: 'background.paper',
        borderRadius: tokens.radius.lg,
      }}
    >
      <ErrorOutlined sx={{ fontSize: 48, color: tokens.color.danger, mb: 1, opacity: 0.8 }} />
      <Typography variant="body1" sx={{ color: 'text.primary', mb: 2 }}>
        {t('common.loadError', { data })}
      </Typography>
      <Button variant="outlined" color="error" onClick={onRetry}>
        {t('common.retry')}
      </Button>
    </Box>
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
    <Box
      sx={{
        p: 4,
        textAlign: 'center',
        backgroundColor: 'background.paper',
        color: 'text.secondary',
        borderRadius: tokens.radius.lg,
      }}
    >
      {icon}
      <Typography variant="h6" sx={{ mb: 1, color: 'text.primary' }}>
        {title}
      </Typography>
      {hint && (
        <Typography variant="body2" sx={{ mb: 2 }}>
          {hint}
        </Typography>
      )}
      {actionLabel && onAction && (
        <Button variant="outlined" onClick={onAction}>
          {actionLabel}
        </Button>
      )}
    </Box>
  );
}
