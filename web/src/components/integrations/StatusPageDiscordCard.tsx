import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  discordChannels,
  type RenderedIntegrationCard,
} from "../../api/integrations";
import {
  previewStatusPageCard,
  retryStatusPageCard,
  statusPageCardStatus,
  type StatusPage,
  type StatusPageSelection,
} from "../../api/statusPages";
import { useCollection } from "../../hooks/useCollection";
import { Button } from "../forms/Button";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { ToggleSwitch } from "../forms/ToggleSwitch";
import { IntegrationCardEmbeds } from "./IntegrationCardEmbeds";
import { CardPublishingStatus } from "./CardPublishingStatus";
import { InlineMessage } from "../status/InlineMessage";

export function StatusPageDiscordCard({
  page,
  title,
  entries,
  channel,
  onChannel,
  disabled,
}: {
  page: StatusPage | null;
  title: string;
  entries: StatusPageSelection[];
  channel: string | null;
  onChannel: (value: string | null) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <FormField label={t("statusPages.discordCard")}>
        <ToggleSwitch
          aria-label={t("statusPages.discordCard")}
          checked={channel !== null}
          onChange={(event) =>
            onChannel(
              event.target.checked ? (page?.discord_channel_id ?? "") : null,
            )
          }
        />
      </FormField>
      {channel !== null && (
        <>
          <PageCardChannel value={channel} onChange={onChannel} />
          {title.trim() &&
            entries.every((entry) => entry.server_id && entry.name.trim()) && (
              <PageCardPreview title={title} entries={entries} />
            )}
        </>
      )}
      {page && page.discord_channel_id !== undefined && (
        <PageCardStatus
          pageID={page.id}
          channel={page.discord_channel_id}
          disabled={disabled}
        />
      )}
    </>
  );
}

function PageCardStatus({
  pageID,
  channel,
  disabled,
}: {
  pageID: string;
  channel: string;
  disabled: boolean;
}) {
  const readStatus = useCallback(
    (signal: AbortSignal) => statusPageCardStatus(pageID, signal),
    [pageID],
  );
  return (
    <CardPublishingStatus
      readStatus={readStatus}
      retry={() => retryStatusPageCard(pageID)}
      revision={channel}
      disabled={disabled}
      showStatus
    />
  );
}

function PageCardChannel({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  const channels = useCollection(discordChannels);
  return (
    <>
      <FormField label={t("serverIntegrations.channel")}>
        <Select
          required
          value={value}
          onValueChange={onChange}
          disabled={channels.loading}
        >
          <option value="">{t("integrations.chooseChannel")}</option>
          {value && !channels.items.some((item) => item.id === value) && (
            <option value={value}>
              {t("serverIntegrations.unavailableChannel", { channel: value })}
            </option>
          )}
          {channels.items.map((item) => (
            <option key={item.id} value={item.id} disabled={!item.available}>
              {item.available
                ? "#" + item.name
                : t("integrations.unavailableChannel", { channel: item.name })}
            </option>
          ))}
        </Select>
      </FormField>
      {channels.error && (
        <InlineMessage role="alert">
          {channels.error}{" "}
          <Button type="button" onClick={() => void channels.refresh()}>
            {t("common.tryAgain")}
          </Button>
        </InlineMessage>
      )}
    </>
  );
}

function PageCardPreview({
  title,
  entries,
}: {
  title: string;
  entries: StatusPageSelection[];
}) {
  const { t } = useTranslation();
  const draft = useMemo(
    () => ({
      title,
      entries: entries.map(({ server_id, name }) => ({ server_id, name })),
    }),
    [title, entries],
  );
  const [result, setResult] = useState<{
    draft: typeof draft;
    preview?: RenderedIntegrationCard;
    error?: string;
  } | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      try {
        const preview = await previewStatusPageCard(
          draft.title,
          draft.entries,
          controller.signal,
        );
        if (!controller.signal.aborted) setResult({ draft, preview });
      } catch (error) {
        if (!controller.signal.aborted)
          setResult({
            draft,
            error:
              error instanceof Error ? error.message : t("statusPages.failed"),
          });
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => void load(), 10000);
    }
    timer = setTimeout(() => void load(), 250);
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [draft, t]);
  const current = result?.draft === draft ? result : null;
  return (
    <section
      className="integration-card-preview"
      aria-label={t("statusPages.discordPreview")}
    >
      <h4>{t("statusPages.discordPreview")}</h4>
      {current?.preview ? (
        <IntegrationCardEmbeds preview={current.preview} />
      ) : current?.error ? (
        <InlineMessage tone="warning">{current.error}</InlineMessage>
      ) : (
        <p className="field-help">{t("common.loading")}</p>
      )}
    </section>
  );
}
