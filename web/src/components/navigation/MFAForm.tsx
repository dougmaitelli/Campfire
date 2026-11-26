import { useId, useRef, useState } from "react";
import { BaseDialog, type BaseDialogProps } from "../layout/BaseDialog";
import { useTranslation } from "react-i18next";
import { beginMFA, confirmMFA, type MFAEnrollment } from "../../api/mfa";
import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { Button } from "../forms/Button";
import { CopyableText } from "../forms/CopyableText";
import { CheckboxField } from "../forms/CheckboxField";
import { InlineMessage } from "../status/InlineMessage";
import "../layout/confirm-dialog.css";
import "../forms/form-stack.css";
import "./mfa.css";

interface MFAFormProps extends Pick<BaseDialogProps, "onClose"> {
  onComplete: () => void;
  onBusy: (value: boolean) => void;
}

export function MFAForm({ onComplete, onClose, onBusy }: MFAFormProps) {
  const { t } = useTranslation();
  const id = useId();
  const [name, setName] = useState("");
  const [enrollment, setEnrollment] = useState<MFAEnrollment | null>(null);
  const [codes, setCodes] = useState<string[]>([]);
  const [code, setCode] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  const cannotClose = busy || (codes.length > 0 && !saved);
  async function submit() {
    if (pending.current) return;
    if (codes.length) {
      if (saved) onComplete();
      return;
    }
    pending.current = true;
    setBusy(true);
    onBusy(true);
    setError(null);
    let unsavedCodes = false;
    try {
      if (enrollment) {
        const issued = await confirmMFA(code);
        setEnrollment(null);
        setCode("");
        if (issued.length) {
          setCodes(issued);
          unsavedCodes = true;
        } else onComplete();
      } else {
        setEnrollment(await beginMFA(name.trim()));
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("mfa.failed"));
    } finally {
      pending.current = false;
      setBusy(false);
      onBusy(unsavedCodes);
    }
  }
  return (
    <BaseDialog
      portal
      busy={cannotClose}
      onClose={() => (codes.length ? onComplete() : onClose())}
      className="confirm-dialog mfa-dialog"
      aria-labelledby={id}
    >
      <form
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <h2 id={id}>{t("mfa.addDevice")}</h2>
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {codes.length ? (
          <>
            <InlineMessage tone="warning">{t("mfa.saveCodes")}</InlineMessage>
            <CopyableText
              value={codes.join("\n")}
              className="mfa-recovery-copy"
              copyLabel={t("mfa.copyCodes")}
              copiedLabel={t("mfa.copied")}
              failedLabel={t("mfa.copyFailed")}
            >
              <pre className="mfa-recovery-codes">{codes.join("\n")}</pre>
            </CopyableText>
            <CheckboxField
              checked={saved}
              onChange={(event) => setSaved(event.target.checked)}
            >
              {t("mfa.savedCodes")}
            </CheckboxField>
          </>
        ) : enrollment ? (
          <>
            <p>{t("mfa.scan")}</p>
            <div className="mfa-setup-material">
              <img
                className="mfa-qr"
                src={enrollment.qr_code}
                width={256}
                height={256}
                alt={t("mfa.qrAlt")}
              />
              <CopyableText
                value={enrollment.secret}
                className="mfa-setup-key"
                copyLabel={t("mfa.copySecret")}
                copiedLabel={t("mfa.copied")}
                failedLabel={t("mfa.copyFailed")}
              >
                <code>{enrollment.secret}</code>
              </CopyableText>
            </div>
            <p className="field-help">
              {t("mfa.setupExpires", {
                time: new Date(
                  enrollment.expires_at * 1000,
                ).toLocaleTimeString(),
              })}
            </p>
            <FormField label={t("mfa.authenticatorCode")}>
              <TextInput
                autoFocus
                autoComplete="one-time-code"
                inputMode="numeric"
                pattern="[0-9]{6}"
                maxLength={6}
                required
                disabled={busy}
                value={code}
                onChange={(event) => setCode(event.target.value)}
              />
            </FormField>
          </>
        ) : (
          <FormField
            label={t("mfa.deviceName")}
            description={t("mfa.deviceNameHelp")}
          >
            <TextInput
              value={name}
              maxLength={100}
              required
              disabled={busy}
              onChange={(event) => setName(event.target.value)}
            />
          </FormField>
        )}
        <div className="confirm-dialog-actions">
          {!codes.length && (
            <Button type="button" disabled={cannotClose} onClick={onClose}>
              {t("common.cancel")}
            </Button>
          )}
          <Button
            type="submit"
            variant="primary"
            disabled={
              busy ||
              (codes.length
                ? !saved
                : enrollment
                  ? !/^[0-9]{6}$/.test(code)
                  : !name.trim())
            }
            loading={busy}
          >
            {codes.length
              ? t("mfa.done")
              : enrollment
                ? t("mfa.confirm")
                : t("mfa.showQR")}
          </Button>
        </div>
      </form>
    </BaseDialog>
  );
}
