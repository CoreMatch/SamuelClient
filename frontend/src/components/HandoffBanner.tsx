import { useEffect, useState } from 'react';
import { Alert, Box, Button, Chip, CircularProgress, Paper, Typography } from '@mui/material';
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
// match summary with players and a live expiry countdown, join now with the
// logged-in account, log in first when needed, retry on failure, and
// readable degradations — the matchmaking result is never dropped silently.
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
  // Ticks once per second while a handoff is pending so the expiry
  // countdown stays live.
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!handoff) return;
    setNow(Date.now());
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [handoff]);

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
  const expiresMs = Date.parse(handoff.expires_at);
  const expired = Number.isFinite(expiresMs) && expiresMs <= now;
  const remaining = formatRemaining(expiresMs - now);

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
          {match.players && match.players.length > 0 && (
            <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap', mt: 1, alignItems: 'center' }}>
              <Typography variant="caption" sx={{ color: 'text.secondary', mr: 0.5 }}>
                {t('handoff.players')}:
              </Typography>
              {match.players.map(player => (
                <Chip
                  key={player.uuid || player.username}
                  size="small"
                  label={player.username}
                  sx={{ height: 20, fontSize: '0.7rem', backgroundColor: 'rgba(255, 255, 255, 0.12)', color: 'text.primary' }}
                />
              ))}
            </Box>
          )}
        </Box>

        <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
          {!expired && hasAccount && (
            <Button
              variant="contained"
              onClick={onJoin}
              disabled={joining}
              startIcon={joining ? <CircularProgress size={16} color="inherit" /> : undefined}
            >
              {joining ? t('handoff.joining') : t('handoff.joinNow')}
            </Button>
          )}
          {!expired && !hasAccount && (
            <Button variant="contained" onClick={onLogin}>
              {t('handoff.login')}
            </Button>
          )}
          <Button variant="outlined" onClick={onDismiss} disabled={joining} sx={{ borderColor: tokens.color.divider, color: 'text.secondary' }}>
            {t('handoff.dismiss')}
          </Button>
        </Box>
      </Box>

      <Box sx={{ display: 'flex', gap: 2, mt: 1, alignItems: 'center', flexWrap: 'wrap' }}>
        {!expired && remaining && (
          <Typography variant="caption" sx={{ color: tokens.color.warning }}>
            {t('handoff.expiresIn', { time: remaining })}
          </Typography>
        )}
        {!hasAccount && !expired && (
          <Typography variant="body2" sx={{ color: tokens.color.warning }}>
            {t('handoff.loginRequired')}
          </Typography>
        )}
      </Box>

      {expired && (
        <Alert severity="warning" sx={{ mt: 1 }}>
          {t('handoff.expired')}
        </Alert>
      )}

      {error && (
        <Alert severity="error" sx={{ mt: 1 }}>
          {error}
        </Alert>
      )}
    </Paper>
  );
}

// formatRemaining renders milliseconds as m:ss (or h:mm:ss), or '' when the
// value is not a positive, finite duration.
function formatRemaining(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '';
  const totalSeconds = Math.floor(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const pad = (n: number) => String(n).padStart(2, '0');
  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds)}` : `${minutes}:${pad(seconds)}`;
}
