import { useTranslation } from "react-i18next";
import { InlineMessage } from "../status/InlineMessage";
import { Button } from "../forms/Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { useId, type ReactNode } from "react";
import { BaseDialog, type BaseDialogProps } from "./BaseDialog";
import "./confirm-dialog.css";

interface ConfirmDialogProps extends Pick<BaseDialogProps, "busy" | "onClose"> {
  title: string;
  description: string;
  confirmLabel: string;
  danger?: boolean;
  warning?: boolean;
  children?: ReactNode;
  disabled?: boolean;
  error: string | null;
  onConfirm: () => void;
}

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
}: ConfirmDialogProps) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <BaseDialog
      busy={busy}
      onClose={onClose}
      initialFocus="[data-dialog-initial-focus]"
      className="confirm-dialog"
      aria-labelledby={`${id}-title`}
      aria-describedby={`${id}-description`}
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
          data-dialog-initial-focus
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
    </BaseDialog>
  );
}
