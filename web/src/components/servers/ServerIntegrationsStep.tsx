import { useTranslation } from "react-i18next";
import type { Server } from "../../api/servers";
import type { ServerIntegrationsState } from "../../hooks/useServerIntegrations";
import { useConfirmation } from "../../hooks/useConfirmation";
import { Button } from "../forms/Button";
import { ToggleSwitch } from "../forms/ToggleSwitch";
import { InlineMessage } from "../status/InlineMessage";
import { Table, TableRow } from "../table/Table";
import { ServerIntegrationFields } from "./ServerIntegrationFields";
import "../../drawers/integrations.css";

export function ServerIntegrationsStep({
  server,
  state,
  busy,
}: {
  server: Server;
  state: ServerIntegrationsState;
  busy: boolean;
}) {
  const { t } = useTranslation();
  const confirmation = useConfirmation();
  const draft = state.items.find((item) => item.key === state.editing);
  const overview = state.overview;
  const channelName = (value: (typeof state.items)[number]["value"]) =>
    state.channels[value.integration_id]?.find(
      (channel) => channel.id === value.channel_id,
    )?.name ?? value.channel_id;

  if (state.loading) return <p role="status">{t("common.loading")}</p>;
  if (state.error)
    return (
      <>
        <InlineMessage role="alert">{state.error}</InlineMessage>
        <Button type="button" onClick={state.retry}>
          {t("serverDrawer.retry")}
        </Button>
      </>
    );
  if (!overview) return null;

  return (
    <>
      {draft ? (
        <>
          <div className="integration-list-heading">
            <Button
              type="button"
              disabled={busy}
              onClick={() => state.setEditing(null)}
            >
              {t("serverIntegrations.backToList")}
            </Button>
            <Button
              type="button"
              variant="danger"
              disabled={busy}
              onClick={() => {
                void confirmation
                  .confirm({
                    title: t("serverIntegrations.removeTitle"),
                    description: t("serverIntegrations.removeHelp"),
                    confirmLabel: t("serverIntegrations.remove"),
                    danger: true,
                  })
                  .then((accepted) => {
                    if (accepted) state.remove(draft.key);
                  });
              }}
            >
              {t("serverIntegrations.remove")}
            </Button>
          </div>
          <ServerIntegrationFields
            key={draft.key}
            server={server}
            availableFields={state.availableFields}
            draft={draft}
            original={state.original(draft.key)}
            overview={overview}
            channels={state.channels}
            channelErrors={state.channelErrors}
            busy={busy}
            onChange={(value, rules) => state.update(draft.key, value, rules)}
          />
        </>
      ) : (
        <>
          <div className="integration-list-heading">
            <Button
              type="button"
              disabled={busy || !overview.available}
              onClick={state.add}
            >
              {t("integrations.add")}
            </Button>
          </div>
          {!overview.configured && (
            <InlineMessage tone="warning">
              {t("serverIntegrations.notConfigured")}
            </InlineMessage>
          )}
          {overview.configured && !overview.available && (
            <InlineMessage tone="warning">
              {t("integrations.disabled")}
            </InlineMessage>
          )}
          <Table
            aria-label={t("serverIntegrations.destinations")}
            empty={!state.items.length}
            emptyMessage={t("serverIntegrations.noDestinations")}
          >
            <thead>
              <tr>
                <th scope="col">{t("serverIntegrations.channel")}</th>
                <th scope="col">{t("serverIntegrations.contents")}</th>
                <th scope="col" className="table-fit">
                  {t("common.enabled")}
                </th>
              </tr>
            </thead>
            <tbody>
              {[...state.items]
                .sort((a, b) =>
                  channelName(a.value).localeCompare(channelName(b.value)),
                )
                .map((item) => {
                  const name = channelName(item.value);
                  const connection = overview.connections.find(
                    (connection) => connection.id === item.value.integration_id,
                  );
                  return (
                    <TableRow
                      key={item.key}
                      onOpen={() => state.setEditing(item.key)}
                    >
                      <td>
                        <button
                          type="button"
                          className="table-row-trigger"
                          aria-label={t("serverIntegrations.editDestination", {
                            channel: name,
                          })}
                          onClick={() => state.setEditing(item.key)}
                        >
                          #{name || t("integrations.chooseChannel")}
                        </button>
                        <span className="row-description">
                          {t("integrations.discord")}
                          {overview.connections.length > 1 && connection
                            ? ` · ${connection.name}`
                            : ""}
                        </span>
                      </td>
                      <td>
                        {[
                          ...item.value.subscriptions.map((subscription) =>
                            t(
                              `serverIntegrations.categories.${subscription.category}`,
                            ),
                          ),
                          ...(item.value.card.enabled
                            ? [t("serverIntegrations.card")]
                            : []),
                        ].join(" · ") || "—"}
                      </td>
                      <td className="table-fit">
                        <ToggleSwitch
                          readOnly
                          disabled
                          aria-label={t("common.enabled")}
                          checked={item.value.enabled}
                        />
                      </td>
                    </TableRow>
                  );
                })}
            </tbody>
          </Table>
        </>
      )}
      {confirmation.dialog}
    </>
  );
}
