import { Alert, Box, Button, CircularProgress, Paper, Typography } from '@mui/material';
import { HandoffPayload } from '../types';
import { t } from '../i18n';

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
    <Paper sx={{ p: 2, mb: 2, backgroundColor: '#282c34', border: '1px solid #4CAF50' }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 2, flexWrap: 'wrap' }}>
        <Box>
          <Typography variant="h6" sx={{ color: 'white' }}>
            {t('handoff.title')}
          </Typography>
          <Typography variant="body2" sx={{ color: '#9ca3af' }}>
            {t('handoff.mode', { mode: match.mode })}
            {match.map_name ? ` · ${t('handoff.map', { map: match.map_name })}` : ''}
          </Typography>
          <Typography variant="body2" sx={{ color: '#9ca3af' }}>
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
              sx={{ backgroundColor: '#4CAF50', '&:hover': { backgroundColor: '#45a049' } }}
            >
              {joining ? t('handoff.joining') : t('handoff.joinNow')}
            </Button>
          ) : (
            <Button variant="contained" onClick={onLogin} sx={{ backgroundColor: '#4CAF50', '&:hover': { backgroundColor: '#45a049' } }}>
              {t('handoff.login')}
            </Button>
          )}
          <Button variant="outlined" onClick={onDismiss} disabled={joining} sx={{ borderColor: '#4a5568', color: '#9ca3af' }}>
            {t('handoff.dismiss')}
          </Button>
        </Box>
      </Box>

      {!hasAccount && (
        <Typography variant="body2" sx={{ color: '#f59e0b', mt: 1 }}>
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
