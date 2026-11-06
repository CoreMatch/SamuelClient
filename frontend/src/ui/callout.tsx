import type { ReactNode } from 'react';
import { CircleAlert, CircleCheck, Info, TriangleAlert, X } from 'lucide-react';
import { IconButton } from './button';

// Callout is the inline alert used for errors, warnings, degradations and
// success notes — one readable line, optional dismiss.

export type CalloutTone = 'info' | 'success' | 'warning' | 'danger';

const toneClasses: Record<CalloutTone, string> = {
  info: 'border-info/30 bg-info/10 text-info',
  success: 'border-ok/30 bg-ok/10 text-ok',
  warning: 'border-warn/30 bg-warn/10 text-warn',
  danger: 'border-danger/30 bg-danger/10 text-danger',
};

const toneIcons: Record<CalloutTone, typeof Info> = {
  info: Info,
  success: CircleCheck,
  warning: TriangleAlert,
  danger: CircleAlert,
};

export function Callout({
  tone,
  children,
  onDismiss,
  dismissLabel = 'Dismiss',
}: {
  tone: CalloutTone;
  children: ReactNode;
  onDismiss?: () => void;
  dismissLabel?: string;
}) {
  const Icon = toneIcons[tone];
  return (
    <div
      role={tone === 'danger' ? 'alert' : 'status'}
      className={`flex items-start gap-3 rounded-md border px-5 py-4 text-sm ${toneClasses[tone]}`}
    >
      <Icon className="mt-0.5 size-5 shrink-0" aria-hidden />
      <div className="min-w-0 flex-1 break-words">{children}</div>
      {onDismiss && (
        <IconButton label={dismissLabel} onClick={onDismiss} className="-my-1 size-6 hover:bg-transparent">
          <X className="size-3.5" />
        </IconButton>
      )}
    </div>
  );
}
