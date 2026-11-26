import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { InlineMessage } from "../status/InlineMessage";
import { Button } from "../forms/Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { useId, useRef, useState, type ReactNode } from "react";
import { formatErrorMessage } from "../../utils/errors";
import { BaseDialog, type BaseDialogProps } from "./BaseDialog";
import "./confirm-dialog.css";
import "./input-dialog.css";

interface InputDialogProps extends Pick<BaseDialogProps, "onClose"> {
  title: string;
  label: string;
  description: string;
  submitLabel: string;
  initialValue?: string;
  busyContent?: ReactNode;
  onCancelBusy?: (() => void) | undefined;
  onSubmit: (value: string) => Promise<void>;
}

export function InputDialog({
  title,
  label,
  description,
  submitLabel,
  initialValue = "",
  busyContent,
  onCancelBusy,
  onClose,
  onSubmit,
}: InputDialogProps) {
  const { t } = useTranslation();
  const pending = useRef(false);
  const id = useId();
  const [value, setValue] = useState(initialValue);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await onSubmit(value.trim());
      onClose();
    } catch (cause) {
      setError(
        formatErrorMessage(
          cause instanceof Error
            ? cause.message
            : i18n.t("inputDialog.unableToCompleteTheRequest"),
        ),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  return (
    <BaseDialog
      portal
      selectInitialText
      busy={busy}
      onClose={onClose}
      className="confirm-dialog input-dialog"
      aria-labelledby={`${id}-title`}
      aria-describedby={`${id}-description`}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
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
        <p id={`${id}-description`}>{description}</p>
        <FormField label={<>{label}</>}>
          <TextInput
            id={`${id}-input`}
            value={value}
            required
            disabled={busy}
            onChange={(event) => setValue(event.target.value)}
          />
        </FormField>
        {busy && busyContent}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        <div className="confirm-dialog-actions">
          <Button
            type="button"

            disabled={busy && !onCancelBusy}
            onClick={() => (busy ? onCancelBusy?.() : onClose())}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            variant="primary"
            disabled={busy || !value.trim()}
            loading={busy}
            loadingLabel={t("inputDialog.working")}
          >
            {submitLabel}
          </Button>
        </div>
      </form>
    </BaseDialog>
  );
}
