import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  previewIntegrationCard,
  type IntegrationCard,
} from "../../api/integrations";
import { InlineMessage } from "../status/InlineMessage";
import { ServerState } from "./ServerState";
import { RuntimeState } from "../../contracts/domains";
import "./integration-card-preview.css";

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
    let timer: ReturnType<typeof setTimeout> | undefined;
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
      if (!controller.signal.aborted)
        timer = setTimeout(() => void load(), 5000);
    }
    void load();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [serverID, selection, key, t]);
  const current = result?.key === key ? result : null;
  const card = current?.card;
  const expired =
    card !== undefined && card.expires_at > 0 && now >= card.expires_at;
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
        <div
          className="integration-server-card"
          data-state={expired ? RuntimeState.Unknown : card.state}
        >
          <strong>{card.name}</strong>
          <dl>
            <div>
              <dt>{t("common.status")}</dt>
              <dd>
                <ServerState
                  state={expired ? RuntimeState.Unknown : card.state}
                />
              </dd>
            </div>
            {card.addresses.length > 0 && (
              <div>
                <dt>{t("serverIntegrations.address")}</dt>
                <dd>{card.addresses.join("\n")}</dd>
              </div>
            )}
            {card.fields.map((field) => (
              <div key={field.id}>
                <dt>{field.name}</dt>
                <dd>
                  {field.available && !expired
                    ? field.value
                    : t("serverIntegrations.valueUnavailable")}
                </dd>
              </div>
            ))}
          </dl>
          <span className="field-help">
            {t("serverIntegrations.previewFooter")}
          </span>
        </div>
      )}
    </section>
  );
}
