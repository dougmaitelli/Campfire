import { useEffect, useId, useRef, type ReactNode } from "react";
import "./confirm-dialog.css";

export function ConfirmDialog({
  title,
  description,
  confirmLabel,
  danger = false,
  warning = false,
  children,
  busy,
  disabled = false,
  error,
  onClose,
  onConfirm,
}: {
  title: string;
  description: string;
  confirmLabel: string;
  danger?: boolean;
  warning?: boolean;
  children?: ReactNode;
  busy: boolean;
  disabled?: boolean;
  error: string | null;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const cancel = useRef<HTMLButtonElement>(null);
  const id = useId();
  useEffect(() => {
    const element = dialog.current;
    element?.showModal();
    cancel.current?.focus();
    return () => element?.close();
  }, []);
  return (
    <dialog
      ref={dialog}
      className="confirm-dialog"
      aria-labelledby={`${id}-title`}
      aria-describedby={`${id}-description`}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <h2 id={`${id}-title`}>{title}</h2>
      <p id={`${id}-description`}>{description}</p>
      {children}
      {error && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
      <div className="confirm-dialog-actions">
        <button
          ref={cancel}
          type="button"
          className="button"
          disabled={busy}
          onClick={onClose}
        >
          Cancel
        </button>
        <button
          type="button"
          className={`button ${danger ? "button-danger" : warning ? "button-warning" : "button-primary"}`}
          disabled={busy || disabled}
          onClick={onConfirm}
        >
          {busy ? "Working…" : confirmLabel}
        </button>
      </div>
    </dialog>
  );
}
