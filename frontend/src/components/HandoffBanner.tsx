import { Alert, Box, Button, CircularProgress, Paper, Typography } from '@mui/material';
import { HandoffPayload } from '../types';
import { t } from '../i18n';
import { tokens } from '../theme';

interface HandoffBannerProps {
  handoff: HandoffPayload | null;
  // Readable degradation notice (handoff gone, pull timeout, version
  // mismatch…) shown when no handoff is pending.
  notice: string;
  hasAccount: boolean;
  joining: boolean;
  error: string;
  onJoin: () => void;
  onLogin: () => void;
  onDismiss: () => void;
}

// HandoffBanner surfaces a pending MatchCore match handoff (INTEROP-SPEC):
// join now with the logged-in account, log in first when needed, retry on
// failure, and show readable degradations — the matchmaking result is never
// dropped silently.
export default function HandoffBanner({
  handoff,
  notice,
  hasAccount,
  joining,
  error,
  onJoin,
  onLogin,
  onDismiss,
}: HandoffBannerProps) {
  if (!handoff && !notice) {
    return null;
  }

  if (!handoff) {
    return (
      <Alert
        severity="info"
        sx={{ mb: 2 }}
        action={
          <Button color="inherit" size="small" onClick={onDismiss}>
            {t('handoff.ok')}
          </Button>
        }
      >
        {notice}
      </Alert>
    );
  }

  const { match } = handoff;

  return (
    <Paper sx={{ p: 2, mb: 2, backgroundColor: 'background.paper', border: `1px solid ${tokens.color.success}`, borderRadius: tokens.radius.lg }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
        <Box>
          <Typography variant="h6" sx={{ color: 'text.primary' }}>
            {t('handoff.title')}
          </Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('handoff.mode', { mode: match.mode })}
            {match.map_name ? ` · ${t('handoff.map', { map: match.map_name })}` : ''}
          </Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary' }}>
            {t('handoff.server', { host: match.join_target.host, port: match.join_target.port })}
          </Typography>
        </Box>

        <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
          {hasAccount ? (
            <Button
              variant="contained"
              onClick={onJoin}
              disabled={joining}
              startIcon={joining ? <CircularProgress size={16} color="inherit" /> : undefined}
            >
              {joining ? t('handoff.joining') : t('handoff.joinNow')}
            </Button>
          ) : (
            <Button variant="contained" onClick={onLogin}>
              {t('handoff.login')}
            </Button>
          )}
          <Button variant="outlined" onClick={onDismiss} disabled={joining} sx={{ borderColor: tokens.color.divider, color: 'text.secondary' }}>
            {t('handoff.dismiss')}
          </Button>
        </Box>
      </Box>

      {!hasAccount && (
        <Typography variant="body2" sx={{ color: tokens.color.warning, mt: 1 }}>
          {t('handoff.loginRequired')}
        </Typography>
      )}

      {error && (
        <Alert severity="error" sx={{ mt: 1 }}>
          {error}
        </Alert>
      )}
    </Paper>
  );
}
