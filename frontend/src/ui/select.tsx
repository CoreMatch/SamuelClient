import * as SelectPrimitive from '@radix-ui/react-select';
import { Check, ChevronDown } from 'lucide-react';

// Select wraps Radix Select with the HA field styling. The trigger keeps the
// native combobox role, so assistive tech (and role-based tests) see a
// standard combobox + option list.

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface SelectProps {
  value: string;
  onValueChange: (value: string) => void;
  options: SelectOption[];
  placeholder?: string;
  disabled?: boolean;
  // Accessible name; also rendered as the visible label when `label` is set
  // by the caller through <Field>.
  ariaLabel?: string;
  className?: string;
}

export function Select({
  value,
  onValueChange,
  options,
  placeholder,
  disabled,
  ariaLabel,
  className = '',
}: SelectProps) {
  return (
    <SelectPrimitive.Root value={value} onValueChange={onValueChange} disabled={disabled}>
      <SelectPrimitive.Trigger
        aria-label={ariaLabel}
        className={`group flex h-9 w-full items-center justify-between gap-2 rounded-md border border-edge bg-raised px-3 text-sm text-ink transition-colors duration-150 ease-standard hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong disabled:pointer-events-none disabled:opacity-40 data-[placeholder]:text-ink-faint ${className}`}
      >
        <SelectPrimitive.Value placeholder={placeholder} />
        <SelectPrimitive.Icon asChild>
          <ChevronDown className="size-4 shrink-0 text-ink-faint transition-transform duration-150 group-data-[state=open]:rotate-180" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Content
          position="popper"
          sideOffset={6}
          className="z-(--ha-z-dropdown) max-h-64 min-w-(--radix-select-trigger-width) overflow-y-auto rounded-md border border-edge-strong bg-raised p-1 shadow-[0_12px_32px_rgba(0,0,0,0.45)]"
        >
          <SelectPrimitive.Viewport>
            {options.map(option => (
              <SelectPrimitive.Item
                key={option.value}
                value={option.value}
                disabled={option.disabled}
                className="relative flex cursor-pointer select-none items-center gap-2 rounded-md px-2.5 py-1.5 text-sm text-ink-muted outline-none data-[disabled]:pointer-events-none data-[highlighted]:bg-white/8 data-[highlighted]:text-ink data-[state=checked]:text-ink"
              >
                <SelectPrimitive.ItemText>{option.label}</SelectPrimitive.ItemText>
                <SelectPrimitive.ItemIndicator className="ml-auto">
                  <Check className="size-3.5" />
                </SelectPrimitive.ItemIndicator>
              </SelectPrimitive.Item>
            ))}
          </SelectPrimitive.Viewport>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  );
}
