import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { resetUserMFA } from "../../api/mfa";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import { useConfirmation } from "../../hooks/useConfirmation";

export function AdminMFAReset({
  userID,
  username,
  onBusy,
}: {
  userID: number;
  username: string;
  onBusy: (value: boolean) => void;
}) {
  const { t } = useTranslation();
  const confirmation = useConfirmation();
  const pending = useRef(false);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function reset() {
    if (pending.current) return;
    pending.current = true;
    if (
      !(await confirmation.confirm({
        title: t("mfa.adminReset"),
        description: t("mfa.adminResetWarning", { username }),
        confirmLabel: t("mfa.adminReset"),
        danger: true,
      }))
    ) {
      pending.current = false;
      return;
    }
    setBusy(true);
    onBusy(true);
    setError(null);
    setDone(false);
    try {
      await resetUserMFA(userID);
      setDone(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : t("mfa.failed"));
    } finally {
      pending.current = false;
      setBusy(false);
      onBusy(false);
    }
  }
  return (
    <section className="form-stack">
      {confirmation.dialog}
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
      {done && (
        <InlineMessage>{t("mfa.adminResetDone", { username })}</InlineMessage>
      )}
      <Button
        type="button"
        variant="danger"
        disabled={busy}
        onClick={() => void reset()}
      >
        {t("mfa.adminReset")}
      </Button>
    </section>
  );
}
