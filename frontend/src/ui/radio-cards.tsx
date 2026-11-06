import * as RadioGroupPrimitive from '@radix-ui/react-radio-group';
import type { ReactNode } from 'react';

// RadioCards turns a Radix RadioGroup into selectable cards (loader choice
// in the install wizard). Graphics first: an icon slot carries the meaning,
// the title is a single word, description/meta are optional extras.

export interface RadioCardOption {
  value: string;
  title: string;
  icon?: ReactNode;
  description?: string;
  meta?: string;
}

export function RadioCards({
  value,
  onValueChange,
  options,
  ariaLabel,
}: {
  value: string;
  onValueChange: (value: string) => void;
  options: RadioCardOption[];
  ariaLabel: string;
}) {
  return (
    <RadioGroupPrimitive.Root
      value={value}
      onValueChange={onValueChange}
      aria-label={ariaLabel}
      className="flex flex-col gap-3"
    >
      {options.map(option => (
        <RadioGroupPrimitive.Item
          key={option.value}
          value={option.value}
          className="w-full rounded-lg border border-edge bg-panel p-5 text-left transition-colors duration-150 ease-standard hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong data-[state=checked]:border-edge-strong data-[state=checked]:bg-white/4"
        >
          <div className="flex items-center gap-4">
            {option.icon && (
              <span className="flex size-11 shrink-0 items-center justify-center rounded-md bg-white/6 text-ink">
                {option.icon}
              </span>
            )}
            <span className="min-w-0 flex-1">
              <span className="block text-base font-semibold text-ink">{option.title}</span>
              {option.description && (
                <span className="mt-1 block text-sm leading-relaxed text-ink-muted">
                  {option.description}
                </span>
              )}
              {option.meta && <span className="mt-1.5 block text-sm text-ink-faint">{option.meta}</span>}
            </span>
            <span
              aria-hidden
              className="size-4 shrink-0 rounded-full border border-edge-strong data-[state=checked]:border-4 data-[state=checked]:border-accent"
              data-state={value === option.value ? 'checked' : 'unchecked'}
            />
          </div>
        </RadioGroupPrimitive.Item>
      ))}
    </RadioGroupPrimitive.Root>
  );
}
