import * as RadioGroupPrimitive from '@radix-ui/react-radio-group';

// RadioCards turns a Radix RadioGroup into selectable cards (used for the
// loader choice in the install wizard).

export interface RadioCardOption {
  value: string;
  title: string;
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
      className="flex flex-col gap-2"
    >
      {options.map(option => (
        <RadioGroupPrimitive.Item
          key={option.value}
          value={option.value}
          className="w-full rounded-lg border border-edge bg-panel p-4 text-left transition-colors duration-150 ease-standard hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong data-[state=checked]:border-edge-strong data-[state=checked]:bg-white/4"
        >
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm font-semibold text-ink">{option.title}</span>
            <span
              aria-hidden
              className="size-3.5 shrink-0 rounded-full border border-edge-strong data-[state=checked]:border-4 data-[state=checked]:border-accent"
              data-state={value === option.value ? 'checked' : 'unchecked'}
            />
          </div>
          {option.description && (
            <p className="mt-1 text-xs leading-relaxed text-ink-muted">{option.description}</p>
          )}
          {option.meta && <p className="mt-1.5 text-xs text-ink-faint">{option.meta}</p>}
        </RadioGroupPrimitive.Item>
      ))}
    </RadioGroupPrimitive.Root>
  );
}
