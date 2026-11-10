import { useTranslation } from "react-i18next";
import { IntegrationLimits } from "../../config/policies";
import type {
  DiscordChannel,
  ServerIntegrationInput,
  ServerIntegrationList,
  ServerIntegrationSettings,
} from "../../api/integrations";
import type { Server } from "../../api/servers";
import type { ServerIntegrationDraft } from "../../hooks/useServerIntegrations";
import { CheckboxField } from "../forms/CheckboxField";
import { Fieldset } from "../forms/Fieldset";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { ToggleSwitch } from "../forms/ToggleSwitch";
import { IntegrationConnectionField } from "../integrations/IntegrationConnectionField";
import { InlineMessage } from "../status/InlineMessage";
import {
  GameEventSubscriptionEditor,
  type GameEventRuleDraft,
} from "./GameEventSubscriptionEditor";
import { IntegrationCardPreview } from "./IntegrationCardPreview";
import { IntegrationCardStatus } from "./IntegrationCardStatus";
import "./server-integrations.css";

export function ServerIntegrationFields({
  server,
  availableFields,
  draft,
  original: current,
  overview,
  channels,
  channelErrors,
  busy,
  onChange,
}: {
  server: Server;
  availableFields: string[];
  draft: ServerIntegrationDraft;
  original: ServerIntegrationSettings | null;
  overview: ServerIntegrationList;
  channels: Record<string, DiscordChannel[]>;
  channelErrors: Record<string, string>;
  busy: boolean;
  onChange: (
    value: ServerIntegrationInput,
    rules?: GameEventRuleDraft[],
  ) => void;
}) {
  const { t } = useTranslation();
  const input = draft.value;
  const connection = overview.connections.find(
    (item) => item.id === input.integration_id,
  );
  const channelChoices = channels[input.integration_id] ?? [];
  const channelError = channelErrors[input.integration_id];
  const selected = channelChoices.find((item) => item.id === input.channel_id);
  const gameSubscription = input.subscriptions.find(
    (subscription) => subscription.category === "game",
  );
  const fields = [
    ...new Set([
      ...availableFields,
      ...(current?.card.fields ?? []),
      ...input.card.fields,
    ]),
  ];
  const locked = busy || !overview.configured;
  return (
    <>
      {!overview.configured && (
        <InlineMessage tone="warning">
          {t("serverIntegrations.notConfigured")}
        </InlineMessage>
      )}
      {connection && !connection.enabled && (
        <InlineMessage tone="warning">
          {t("integrations.disabled")}
        </InlineMessage>
      )}
      {channelError && (
        <InlineMessage tone="warning">{channelError}</InlineMessage>
      )}
      <IntegrationConnectionField
        connections={overview.connections}
        value={input.integration_id}
        disabled={locked}
        readOnly={!!current}
        onChange={(integration_id) =>
          onChange({ ...input, integration_id, channel_id: "" })
        }
      />
      {connection && (
        <>
          <FormField
            label={t("serverIntegrations.channel")}
            description={
              selected && !selected.available
                ? t("integrations.missing", {
                    permissions: selected.missing.join(", "),
                  })
                : undefined
            }
          >
            <Select
              value={input.channel_id}
              disabled={locked || !connection.enabled}
              onValueChange={(channel_id) => onChange({ ...input, channel_id })}
            >
              <option value="">{t("integrations.chooseChannel")}</option>
              {input.channel_id && !selected && (
                <option value={input.channel_id}>
                  {t("serverIntegrations.unavailableChannel", {
                    channel: input.channel_id,
                  })}
                </option>
              )}
              {channelChoices.map((item) => (
                <option key={item.id} value={item.id}>
                  {item.available
                    ? `#${item.name}`
                    : t("integrations.unavailableChannel", {
                        channel: item.name,
                      })}
                </option>
              ))}
            </Select>
          </FormField>
          <FormField label={t("common.enabled")}>
            <ToggleSwitch
              aria-label={t("common.enabled")}
              checked={input.enabled}
              disabled={locked || (!input.enabled && !connection.enabled)}
              onChange={(event) =>
                onChange({ ...input, enabled: event.target.checked })
              }
            />
          </FormField>
          <Fieldset className="server-integration-options" disabled={locked}>
            <legend>{t("serverIntegrations.notifications")}</legend>
            {overview.sources.map((source) => {
              const checked = input.subscriptions.some(
                (subscription) => subscription.category === source.id,
              );
              const allowed = server.permissions.includes(source.permission);
              return (
                <CheckboxField
                  key={source.id}
                  checked={checked}
                  disabled={!checked && !allowed}
                  onChange={(event) =>
                    onChange({
                      ...input,
                      subscriptions: event.target.checked
                        ? [...input.subscriptions, { category: source.id }]
                        : input.subscriptions.filter(
                            (subscription) =>
                              subscription.category !== source.id,
                          ),
                    })
                  }
                >
                  {t(`serverIntegrations.categories.${source.id}`)}
                  {!allowed &&
                    ` (${t("serverIntegrations.permissionRequired")})`}
                </CheckboxField>
              );
            })}
            {gameSubscription && (
              <GameEventSubscriptionEditor
                key={server.id}
                serverID={server.id}
                value={gameSubscription}
                rules={draft.rules}
                onChange={(value, rules) =>
                  onChange(
                    {
                      ...input,
                      subscriptions: input.subscriptions.map((subscription) =>
                        subscription.category === "game" ? value : subscription,
                      ),
                    },
                    rules,
                  )
                }
              />
            )}
          </Fieldset>
          <Fieldset className="server-integration-options" disabled={locked}>
            <legend>{t("serverIntegrations.card")}</legend>
            <CheckboxField
              checked={input.card.enabled}
              onChange={(event) =>
                onChange({
                  ...input,
                  card: {
                    ...input.card,
                    enabled: event.target.checked,
                  },
                })
              }
            >
              {t("serverIntegrations.showCard")}
            </CheckboxField>
            <p className="field-help">{t("serverIntegrations.cardHelp")}</p>
            {input.card.enabled && (
              <>
                {fields.map((field) => (
                  <CheckboxField
                    key={field}
                    checked={input.card.fields.includes(field)}
                    disabled={
                      !input.card.fields.includes(field) &&
                      input.card.fields.length >= IntegrationLimits.CardFields
                    }
                    onChange={(event) =>
                      onChange({
                        ...input,
                        card: {
                          ...input.card,
                          fields: event.target.checked
                            ? [...input.card.fields, field]
                            : input.card.fields.filter(
                                (value) => value !== field,
                              ),
                        },
                      })
                    }
                  >
                    {field.startsWith("field.") ? field.slice(6) : field}
                  </CheckboxField>
                ))}
                <p className="field-help">
                  {t("serverIntegrations.fieldsHelp")}
                </p>
                <IntegrationCardPreview
                  serverID={server.id}
                  fields={input.card.fields}
                />
              </>
            )}
            {input.enabled &&
              input.card.enabled &&
              connection.enabled &&
              (!current?.enabled || !current.card.enabled) && (
                <p className="field-help">
                  {t("serverIntegrations.saveToPublish")}
                </p>
              )}
            {current && (
              <IntegrationCardStatus
                serverID={server.id}
                bindingID={current.id}
                revision={JSON.stringify(current)}
                disabled={busy}
                showStatus={input.enabled && input.card.enabled}
              />
            )}
          </Fieldset>
        </>
      )}
    </>
  );
}
