import { IntegrationCardState } from "../../contracts/domains";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { IntegrationCardStatus as CardStatus } from "../../api/integrations";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import { StatusPill } from "../status/StatusPill";

export function CardStatusPill({ state }: { state: CardStatus["state"] }) {
  const { t } = useTranslation();
  const tones = {
    published: "positive",
    failed: "negative",
    uncertain: "warning",
    pending: "pending",
    disabled: "neutral",
  } as const;
  return (
    <StatusPill tone={tones[state]}>
      {t(`serverIntegrations.cardStates.${state}`)}
    </StatusPill>
  );
}

export function CardRetryButton({
  status,
  disabled,
  onClick,
}: {
  status: CardStatus;
  disabled: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  if (!status.can_retry) return null;
  return (
    <Button type="button" disabled={disabled} onClick={onClick}>
      {t(
        status.state === IntegrationCardState.Uncertain
          ? "serverIntegrations.checkCard"
          : "serverIntegrations.retryCard",
      )}
    </Button>
  );
}

export function CardPublishingStatus({
  readStatus,
  retry,
  revision,
  disabled,
  showStatus,
}: {
  readStatus: (signal: AbortSignal) => Promise<CardStatus>;
  retry: () => Promise<void>;
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
        const result = await readStatus(controller.signal);
        if (!controller.signal.aborted) {
          setStatus(result);
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
  }, [readStatus, revision, refresh, t]);
  const message = error ?? status?.error;
  if (
    !message &&
    (!showStatus || !status || status.state === IntegrationCardState.Disabled)
  )
    return null;
  return (
    <section aria-label={t("serverIntegrations.publishing")}>
      <h4>{t("serverIntegrations.publishing")}</h4>
      {status && <CardStatusPill state={status.state} />}
      {message && <InlineMessage tone="warning">{message}</InlineMessage>}
      {status && (
        <CardRetryButton
          status={status}
          disabled={disabled || busy}
          onClick={() => {
            if (pending.current) return;
            pending.current = true;
            setBusy(true);
            void retry()
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
        />
      )}
    </section>
  );
}
