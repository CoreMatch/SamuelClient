import { forwardRef } from 'react';
import type { ButtonHTMLAttributes, ReactNode } from 'react';
import { LoaderCircle } from 'lucide-react';

// Button styles shared across the app. Variants stay within the HA token
// palette: primary is the neutral-white CTA, danger is reserved for
// destructive acts, semantic hues never decorate plain actions.

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';
type Size = 'sm' | 'md' | 'lg';

const variantClasses: Record<Variant, string> = {
  primary:
    'border border-transparent bg-accent text-on-accent hover:bg-accent-hover',
  secondary:
    'border border-edge-strong bg-transparent text-ink hover:bg-white/6',
  ghost:
    'border border-transparent bg-transparent text-ink-muted hover:bg-white/6 hover:text-ink',
  danger:
    'border border-transparent bg-danger/10 text-danger hover:bg-danger/20',
};

const sizeClasses: Record<Size, string> = {
  sm: 'h-9 gap-1.5 px-3.5 text-sm',
  md: 'h-11 gap-2 px-5 text-base',
  lg: 'h-13 gap-2.5 px-7 text-base',
};

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  loading?: boolean;
  children: ReactNode;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'primary', size = 'md', loading = false, disabled, className = '', children, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      disabled={disabled || loading}
      className={`inline-flex select-none items-center justify-center rounded-md font-medium transition-colors duration-150 ease-standard focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong disabled:pointer-events-none disabled:opacity-40 ${variantClasses[variant]} ${sizeClasses[size]} ${className}`}
      {...rest}
    >
      {loading && <LoaderCircle className="size-5 animate-spin" aria-hidden />}
      {children}
    </button>
  );
});

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string;
  children: ReactNode;
  tone?: 'default' | 'danger';
}

// IconButton is a square ghost button for icon-only actions. Pair it with the
// Tooltip wrapper for a readable label on hover.
export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { label, tone = 'default', className = '', children, ...rest },
  ref,
) {
  const toneClass =
    tone === 'danger'
      ? 'text-ink-faint hover:bg-danger/10 hover:text-danger'
      : 'text-ink-faint hover:bg-white/6 hover:text-ink';
  return (
    <button
      ref={ref}
      type="button"
      aria-label={label}
      title={label}
      className={`inline-flex size-9 items-center justify-center rounded-md transition-colors duration-150 ease-standard focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong disabled:pointer-events-none disabled:opacity-40 ${toneClass} ${className}`}
      {...rest}
    >
      {children}
    </button>
  );
});
