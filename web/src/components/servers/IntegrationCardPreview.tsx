import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  previewIntegrationCard,
  type IntegrationCard,
} from "../../api/integrations";
import { RuntimeState } from "../../contracts/domains";
import { IntegrationCardEmbeds } from "../integrations/IntegrationCardEmbeds";
import { InlineMessage } from "../status/InlineMessage";

export function IntegrationCardPreview({
  serverID,
  fields,
}: {
  serverID: string;
  fields: string[];
}) {
  const { t } = useTranslation();
  const selection = JSON.stringify(fields);
  const key = `${serverID}:${selection}`;
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));
  useEffect(() => {
    const timer = setInterval(
      () => setNow(Math.floor(Date.now() / 1000)),
      1000,
    );
    return () => clearInterval(timer);
  }, []);
  const [result, setResult] = useState<{
    key: string;
    card?: IntegrationCard;
    error?: string;
  } | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try {
        const card = await previewIntegrationCard(
          serverID,
          JSON.parse(selection) as string[],
          controller.signal,
        );
        if (!controller.signal.aborted) setResult({ key, card });
      } catch (cause) {
        if (!controller.signal.aborted)
          setResult({
            key,
            error:
              cause instanceof Error ? cause.message : t("integrations.failed"),
          });
      }
    }
    void load();
    return () => {
      controller.abort();
    };
  }, [serverID, selection, key, t]);
  const current = result?.key === key ? result : null;
  const card = current?.card;
  const expired =
    card !== undefined && card.expires_at > 0 && now >= card.expires_at;
  const state = expired
    ? RuntimeState.Unknown
    : (card?.state ?? RuntimeState.Unknown);
  const preview = expired ? card.expired_preview : card?.preview;
  return (
    <section
      className="integration-card-preview"
      aria-label={t("serverIntegrations.preview")}
    >
      <h4>{t("serverIntegrations.preview")}</h4>
      {current?.error ? (
        <InlineMessage tone="error">{current.error}</InlineMessage>
      ) : !card ? (
        <p className="field-help">{t("common.loading")}</p>
      ) : (
        preview && <IntegrationCardEmbeds preview={preview} state={state} />
      )}
    </section>
  );
}
