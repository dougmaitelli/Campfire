import { useTranslation } from "react-i18next";
import { InlineMessage } from "../status/InlineMessage";
import { Button } from "../forms/Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
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
  const { t } = useTranslation();
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
      <div className="confirm-dialog-header">
        <h2 id={`${id}-title`}>{title}</h2>
        <Button
          type="button"
          iconOnly
          className="confirm-dialog-close"
          aria-label={t("common.close")}
          disabled={busy}
          onClick={onClose}
        >
          <NavigationIcon name="close" />
        </Button>
      </div>
      <div className="confirm-dialog-description">
        {(danger || warning) && (
          <span
            className={`confirm-dialog-indicator${danger ? " confirm-dialog-indicator-danger" : ""}`}
          >
            <NavigationIcon name="warning" />
          </span>
        )}
        <p id={`${id}-description`}>{description}</p>
      </div>
      {children}
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
      <div className="confirm-dialog-actions">
        <Button
          ref={cancel}
          type="button"

          disabled={busy}
          onClick={onClose}
        >
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          variant={danger ? "danger" : "primary"}
          disabled={busy || disabled}
          onClick={onConfirm}
          loading={busy}
          loadingLabel={t("confirmDialog.working")}
        >
          {confirmLabel}
        </Button>
      </div>
    </dialog>
  );
}
