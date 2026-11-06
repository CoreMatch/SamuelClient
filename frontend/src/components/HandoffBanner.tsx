import { useEffect, useState } from 'react';
import { Clock } from 'lucide-react';
import type { HandoffPayload } from '../types';
import { t } from '../i18n';
import { Button } from '../ui/button';
import { Badge } from '../ui/badge';
import { Callout } from '../ui/callout';

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
      <div className="mb-6 flex items-center gap-3">
        <div className="min-w-0 flex-1">
          <Callout tone="info">{notice}</Callout>
        </div>
        <Button variant="ghost" size="sm" onClick={onDismiss}>
          {t('handoff.ok')}
        </Button>
      </div>
    );
  }

  const { match } = handoff;
  const expiresMs = Date.parse(handoff.expires_at);
  const expired = Number.isFinite(expiresMs) && expiresMs <= now;
  const remaining = formatRemaining(expiresMs - now);

  return (
    <section className="mb-6 rounded-lg border border-ok/40 bg-panel">
      <div className="flex flex-wrap items-start justify-between gap-4 p-5">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="size-2 rounded-full bg-success" aria-hidden />
            <h2 className="text-base font-semibold text-ink">{t('handoff.title')}</h2>
          </div>
          <p className="mt-1.5 text-sm text-ink-muted">
            {t('handoff.mode', { mode: match.mode })}
            {match.map_name ? ` · ${t('handoff.map', { map: match.map_name })}` : ''}
          </p>
          <p className="mt-0.5 font-mono text-xs text-ink-muted">
            {t('handoff.server', { host: match.join_target.host, port: match.join_target.port })}
          </p>
          {match.players && match.players.length > 0 && (
            <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
              <span className="text-xs text-ink-faint">{t('handoff.players')}:</span>
              {match.players.map(player => (
                <Badge key={player.uuid || player.username}>{player.username}</Badge>
              ))}
            </div>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-2">
          {!expired && hasAccount && (
            <Button onClick={onJoin} disabled={joining} loading={joining}>
              {joining ? t('handoff.joining') : t('handoff.joinNow')}
            </Button>
          )}
          {!expired && !hasAccount && (
            <Button onClick={onLogin}>{t('handoff.login')}</Button>
          )}
          <Button variant="secondary" onClick={onDismiss} disabled={joining}>
            {t('handoff.dismiss')}
          </Button>
        </div>
      </div>

      {!expired && (remaining || !hasAccount) && (
        <div className="flex flex-wrap items-center gap-4 border-t border-edge px-5 py-2.5">
          {remaining && (
            <span className="inline-flex items-center gap-1.5 font-mono text-xs text-warn">
              <Clock className="size-3.5" aria-hidden />
              {t('handoff.expiresIn', { time: remaining })}
            </span>
          )}
          {!hasAccount && (
            <span className="text-xs text-warn">{t('handoff.loginRequired')}</span>
          )}
        </div>
      )}

      {(expired || error) && (
        <div className="flex flex-col gap-2 border-t border-edge px-5 py-3">
          {expired && <Callout tone="warning">{t('handoff.expired')}</Callout>}
          {error && <Callout tone="danger">{error}</Callout>}
        </div>
      )}
    </section>
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
