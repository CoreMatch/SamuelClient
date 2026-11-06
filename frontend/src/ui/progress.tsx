import * as ProgressPrimitive from '@radix-ui/react-progress';
import * as SliderPrimitive from '@radix-ui/react-slider';

// Progress is the install/join progress bar. `value == null` renders an
// indeterminate pulse instead of a fake percentage.
export function Progress({ value }: { value: number | null }) {
  return (
    <ProgressPrimitive.Root
      value={value ?? undefined}
      className="relative h-2 w-full overflow-hidden rounded-full bg-white/8"
    >
      <ProgressPrimitive.Indicator
        className="h-full rounded-full bg-accent transition-transform duration-200 ease-standard"
        style={
          value == null
            ? { width: '100%', transform: 'translateX(0)', animation: 'ha-progress-pulse 1.4s var(--ha-ease) infinite' }
            : { width: `${value}%`, transform: 'translateX(0)' }
        }
      />
      <style>{`@keyframes ha-progress-pulse { 0% { opacity: 0.35; } 50% { opacity: 0.9; } 100% { opacity: 0.35; } }`}</style>
    </ProgressPrimitive.Root>
  );
}

// Slider wraps Radix Slider with the HA track/thumb styling.
export function Slider({
  value,
  onValueChange,
  min,
  max,
  step,
  ariaLabel,
}: {
  value: number;
  onValueChange: (value: number) => void;
  min: number;
  max: number;
  step: number;
  ariaLabel: string;
}) {
  return (
    <SliderPrimitive.Root
      value={[value]}
      onValueChange={next => onValueChange(next[0])}
      min={min}
      max={max}
      step={step}
      aria-label={ariaLabel}
      className="relative flex h-6 w-full touch-none select-none items-center"
    >
      <SliderPrimitive.Track className="relative h-2 grow rounded-full bg-white/10">
        <SliderPrimitive.Range className="absolute h-full rounded-full bg-accent" />
      </SliderPrimitive.Track>
      <SliderPrimitive.Thumb
        className="block size-5 rounded-full border border-edge-strong bg-ink shadow-[0_2px_8px_rgba(0,0,0,0.4)] transition-transform duration-150 hover:scale-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong"
        aria-label={ariaLabel}
      />
    </SliderPrimitive.Root>
  );
}
