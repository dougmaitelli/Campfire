import { InlineMessage } from "../status/InlineMessage";
import { Button } from "../forms/Button";
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
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
      <div className="confirm-dialog-actions">
        <Button
          ref={cancel}
          type="button"

          disabled={busy}
          onClick={onClose}
        >
          Cancel
        </Button>
        <Button
          type="button"
          variant={danger ? "danger" : warning ? "warning" : "primary"}
          disabled={busy || disabled}
          onClick={onConfirm}
          loading={busy}
          loadingLabel={"Working…"}
        >
          {confirmLabel}
        </Button>
      </div>
    </dialog>
  );
}
