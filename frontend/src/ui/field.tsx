import { useId } from 'react';
import type { InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from 'react';

// Field pairs a visible label with a form control. The label is associated
// via htmlFor/id so getByLabel-style queries and screen readers resolve it.

const inputClasses =
  'w-full rounded-md border border-edge bg-raised px-4 text-base text-ink placeholder:text-ink-faint transition-colors duration-150 ease-standard hover:border-edge-strong focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-edge-strong disabled:pointer-events-none disabled:opacity-40';

export function Field({
  label,
  htmlFor,
  children,
  className = '',
}: {
  label: string;
  htmlFor: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={`flex flex-col gap-2 ${className}`}>
      <label htmlFor={htmlFor} className="text-sm font-medium text-ink-muted">
        {label}
      </label>
      {children}
    </div>
  );
}

export interface TextFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
}

export function TextField({ label, id, className = '', ...rest }: TextFieldProps) {
  const autoId = useId();
  const inputId = id ?? autoId;
  return (
    <Field label={label} htmlFor={inputId} className={className}>
      <input id={inputId} className={`${inputClasses} h-11`} {...rest} />
    </Field>
  );
}

export interface NumberFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string;
}

export function NumberField({ label, id, className = '', ...rest }: NumberFieldProps) {
  const autoId = useId();
  const inputId = id ?? autoId;
  return (
    <Field label={label} htmlFor={inputId} className={className}>
      <input id={inputId} type="number" className={`${inputClasses} h-11 font-mono`} {...rest} />
    </Field>
  );
}

export interface TextAreaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label: string;
}

export function TextArea({ label, id, className = '', ...rest }: TextAreaProps) {
  const autoId = useId();
  const inputId = id ?? autoId;
  return (
    <Field label={label} htmlFor={inputId} className={className}>
      <textarea id={inputId} className={`${inputClasses} min-h-20 py-2.5 font-mono text-sm`} {...rest} />
    </Field>
  );
}
