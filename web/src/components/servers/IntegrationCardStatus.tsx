import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  serverDiscordSettings,
  retryIntegrationCard,
  type IntegrationCardStatus as CardStatus,
} from "../../api/integrations";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import { StatusPill } from "../status/StatusPill";

export function IntegrationCardStatus({
  serverID,
  bindingID,
  revision,
  disabled,
  showStatus,
}: {
  serverID: string;
  bindingID: string;
  revision: string;
  disabled: boolean;
  showStatus: boolean;
}) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<CardStatus | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const pending = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      try {
        const result = await serverDiscordSettings(
          serverID,
          bindingID,
          controller.signal,
        );
        if (!controller.signal.aborted) {
          setStatus(result.card_status);
          setError(null);
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : t("integrations.failed"),
          );
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => void load(), 5000);
    }
    void load();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [serverID, bindingID, revision, refresh, t]);
  const message = error ?? status?.error;
  if (!message && (!showStatus || !status || status.state === "disabled"))
    return null;
  return (
    <section aria-label={t("serverIntegrations.publishing")}>
      <h4>{t("serverIntegrations.publishing")}</h4>
      {status && (
        <StatusPill
          tone={
            status.state === "published"
              ? "positive"
              : status.state === "failed"
                ? "negative"
                : status.state === "uncertain"
                  ? "warning"
                  : status.state === "pending"
                    ? "pending"
                    : "neutral"
          }
        >
          {t(`serverIntegrations.cardStates.${status.state}`)}
        </StatusPill>
      )}
      {message && <InlineMessage tone="warning">{message}</InlineMessage>}
      {status?.can_retry && (
        <Button
          disabled={disabled || busy}
          onClick={() => {
            if (pending.current) return;
            pending.current = true;
            setBusy(true);
            void retryIntegrationCard(serverID, bindingID)
              .then(() => setRefresh((value) => value + 1))
              .catch((cause: unknown) =>
                setError(
                  cause instanceof Error
                    ? cause.message
                    : t("integrations.failed"),
                ),
              )
              .finally(() => {
                pending.current = false;
                setBusy(false);
              });
          }}
        >
          {t(
            status.state === "uncertain"
              ? "serverIntegrations.checkCard"
              : "serverIntegrations.retryCard",
          )}
        </Button>
      )}
    </section>
  );
}
