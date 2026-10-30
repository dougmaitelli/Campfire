import { useState } from "react";
import { useTranslation } from "react-i18next";
import { stopViewAs } from "../../api/auth";
import { useAuthentication } from "../../hooks/useAuthentication";
import { formatUserLabel } from "../../utils/formatUserLabel";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import "./view-as-banner.css";

export function ViewAsBanner() {
  const { t } = useTranslation();
  const { session } = useAuthentication();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (!session?.view_as) {
    return session?.view_as_ended ? (
      <InlineMessage tone="info">{t("viewAs.ended")}</InlineMessage>
    ) : null;
  }
  async function leave() {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await stopViewAs();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("viewAs.unableToStop"),
      );
      setBusy(false);
    }
  }
  return (
    <section className="view-as-banner" aria-label={t("viewAs.preview")}>
      <div>
        <strong>
          {t("viewAs.viewingAs", {
            user: formatUserLabel({
              name: session.name ?? "",
              username: session.username ?? "",
            }),
          })}
        </strong>
        <p>{t("viewAs.readOnly")}</p>
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
      </div>
      <Button
        variant="primary"
        disabled={busy}
        onClick={() => {
          void leave();
        }}
      >
        {t("viewAs.returnToAdmin")}
      </Button>
    </section>
  );
}
