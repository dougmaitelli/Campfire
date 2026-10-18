import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { InlineMessage } from "../status/InlineMessage";
import { Button } from "../forms/Button";
import { useEffect, useId, useRef, useState } from "react";
import { formatErrorMessage } from "../../utils/errors";
import "./confirm-dialog.css";
import "./input-dialog.css";

export function InputDialog({
  title,
  label,
  description,
  submitLabel,
  initialValue = "",
  onClose,
  onSubmit,
}: {
  title: string;
  label: string;
  description: string;
  submitLabel: string;
  initialValue?: string;
  onClose: () => void;
  onSubmit: (value: string) => Promise<void>;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const pending = useRef(false);
  const id = useId();
  const [value, setValue] = useState(initialValue);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    const element = dialog.current;
    element?.showModal();
    input.current?.focus();
    input.current?.select();
    return () => element?.close();
  }, []);

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
            : "Unable to complete the request.",
        ),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }

  return (
    <dialog
      ref={dialog}
      className="confirm-dialog input-dialog"
      aria-labelledby={`${id}-title`}
      aria-describedby={`${id}-description`}
      onCancel={(event) => {
        event.preventDefault();
        if (!pending.current) onClose();
      }}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <h2 id={`${id}-title`}>{title}</h2>
        <p id={`${id}-description`}>{description}</p>
        <FormField label={<>{label}</>}>
          <TextInput
            ref={input}
            id={`${id}-input`}
            value={value}
            required
            disabled={busy}
            onChange={(event) => setValue(event.target.value)}
          />
        </FormField>
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        <div className="confirm-dialog-actions">
          <Button
            type="button"

            disabled={busy}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            type="submit"
            variant="primary"
            disabled={busy || !value.trim()}
            loading={busy}
            loadingLabel={"Working…"}
          >
            {submitLabel}
          </Button>
        </div>
      </form>
    </dialog>
  );
}
