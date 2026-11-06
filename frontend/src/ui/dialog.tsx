import * as DialogPrimitive from '@radix-ui/react-dialog';
import * as AlertDialogPrimitive from '@radix-ui/react-alert-dialog';
import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { IconButton } from './button';

// Modal surfaces: Dialog for forms (add/edit Java), AlertDialog for
// destructive confirmations (delete account / Java). Both share the overlay
// and panel styling.

const overlayClasses = 'fixed inset-0 z-(--ha-z-overlay) bg-black/60 backdrop-blur-[2px]';
const panelClasses =
  'w-full max-w-md rounded-lg border border-edge bg-panel p-6 shadow-[0_24px_64px_rgba(0,0,0,0.5)]';

export function Dialog({
  open,
  onOpenChange,
  title,
  children,
  closeLabel,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  children: ReactNode;
  closeLabel: string;
}) {
  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className={overlayClasses} />
        <DialogPrimitive.Content className="fixed inset-0 z-(--ha-z-overlay) grid place-items-center p-4">
          <div className={panelClasses}>
            <div className="mb-4 flex items-center justify-between">
              <DialogPrimitive.Title className="text-base font-semibold text-ink">
                {title}
              </DialogPrimitive.Title>
              <DialogPrimitive.Close asChild>
                <IconButton label={closeLabel}>
                  <X className="size-4" />
                </IconButton>
              </DialogPrimitive.Close>
            </div>
            {children}
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

export function AlertDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  confirmLabel: string;
  cancelLabel: string;
  onConfirm: () => void;
}) {
  return (
    <AlertDialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <AlertDialogPrimitive.Portal>
        <AlertDialogPrimitive.Overlay className={overlayClasses} />
        <AlertDialogPrimitive.Content className="fixed inset-0 z-(--ha-z-overlay) grid place-items-center p-4">
          <div className={panelClasses}>
            <AlertDialogPrimitive.Title className="text-base font-semibold text-ink">
              {title}
            </AlertDialogPrimitive.Title>
            <AlertDialogPrimitive.Description className="mt-2 text-sm leading-relaxed text-ink-muted">
              {description}
            </AlertDialogPrimitive.Description>
            <div className="mt-6 flex justify-end gap-2">
              <AlertDialogPrimitive.Cancel asChild>
                <button
                  type="button"
                  className="inline-flex h-9 items-center justify-center rounded-md border border-edge-strong px-4 text-sm font-medium text-ink transition-colors duration-150 hover:bg-white/6"
                >
                  {cancelLabel}
                </button>
              </AlertDialogPrimitive.Cancel>
              <AlertDialogPrimitive.Action asChild>
                <button
                  type="button"
                  onClick={onConfirm}
                  className="inline-flex h-9 items-center justify-center rounded-md bg-danger px-4 text-sm font-medium text-white transition-colors duration-150 hover:bg-danger/85"
                >
                  {confirmLabel}
                </button>
              </AlertDialogPrimitive.Action>
            </div>
          </div>
        </AlertDialogPrimitive.Content>
      </AlertDialogPrimitive.Portal>
    </AlertDialogPrimitive.Root>
  );
}
