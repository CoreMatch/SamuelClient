import type { ReactNode } from 'react';

// Badge is the pill used for version types, auth modes and player chips.
// Tones map 1:1 onto the semantic palette.

export type BadgeTone = 'neutral' | 'success' | 'warning' | 'danger' | 'info';

const toneClasses: Record<BadgeTone, string> = {
  neutral: 'bg-white/8 text-ink-muted',
  success: 'bg-ok/10 text-ok',
  warning: 'bg-warn/10 text-warn',
  danger: 'bg-danger/10 text-danger',
  info: 'bg-info/10 text-info',
};

export function Badge({
  tone = 'neutral',
  children,
  className = '',
}: {
  tone?: BadgeTone;
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-sm font-medium ${toneClasses[tone]} ${className}`}
    >
      {children}
    </span>
  );
}
