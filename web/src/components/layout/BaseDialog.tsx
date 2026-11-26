import { useEffect, useRef, type ComponentPropsWithoutRef } from "react";
import { createPortal } from "react-dom";

export interface BaseDialogProps extends Omit<
  ComponentPropsWithoutRef<"dialog">,
  "open" | "onCancel" | "onClose" | "onClick"
> {
  onClose: () => void;
  busy: boolean;
  open?: boolean;
  portal?: boolean;
  initialFocus?: string;
  selectInitialText?: boolean;
  dismissOnBackdrop?: boolean;
}

// Specialized dialogs share native modality, focus, and dismissal behavior.
export function BaseDialog({
  onClose,
  busy,
  open = true,
  portal = false,
  initialFocus = "input, textarea",
  selectInitialText = false,
  dismissOnBackdrop = false,
  ...props
}: BaseDialogProps) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current;
    if (!dialog || !open) return;
    dialog.showModal();
    const target = dialog.querySelector<HTMLElement>(initialFocus);
    target?.focus();
    if (
      selectInitialText &&
      (target instanceof HTMLInputElement ||
        target instanceof HTMLTextAreaElement)
    )
      target.select();
    return () => dialog.close();
  }, [open, initialFocus, selectInitialText]);
  const dialog = (
    <dialog
      {...props}
      ref={ref}
      onCancel={(event) => {
        event.preventDefault();
        event.stopPropagation();
        if (!busy) onClose();
      }}
      onClose={(event) => {
        event.stopPropagation();
      }}
      onClick={(event) => {
        event.stopPropagation();
        if (dismissOnBackdrop && !busy && event.target === event.currentTarget)
          onClose();
      }}
    />
  );
  return portal ? createPortal(dialog, document.body) : dialog;
}
