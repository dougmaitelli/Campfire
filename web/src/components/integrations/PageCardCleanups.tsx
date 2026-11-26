import { useTranslation } from "react-i18next";
import type { DiscordChannel } from "../../api/integrations";
import {
  listPageCardCleanups,
  retryStatusPageCard,
} from "../../api/statusPages";
import { useCollection } from "../../hooks/useCollection";
import { InlineMessage } from "../status/InlineMessage";
import { Table } from "../table/Table";
import { CardRetryButton, CardStatusPill } from "./CardPublishingStatus";

export function PageCardCleanups({
  channels,
  busy,
  perform,
}: {
  channels: DiscordChannel[];
  busy: boolean;
  perform: (action: () => Promise<void>) => Promise<void>;
}) {
  const { t } = useTranslation();
  const cleanups = useCollection(listPageCardCleanups, 5000);
  if (!cleanups.items.length && !cleanups.error) return null;
  return (
    <section aria-label={t("integrations.pageCardCleanups")}>
      <h3>{t("integrations.pageCardCleanups")}</h3>
      {cleanups.error && (
        <InlineMessage role="alert">{cleanups.error}</InlineMessage>
      )}
      {cleanups.items.length > 0 && (
        <Table>
          <thead>
            <tr>
              <th scope="col">{t("statusPages.title")}</th>
              <th scope="col">{t("common.status")}</th>
              <th scope="col" className="table-actions">
                {t("common.actions")}
              </th>
            </tr>
          </thead>
          <tbody>
            {cleanups.items.map((row) => (
              <tr key={row.id}>
                <td>
                  {row.title || row.id}
                  <span className="row-description">
                    #
                    {channels.find((channel) => channel.id === row.channel_id)
                      ?.name ?? row.channel_id}
                  </span>
                </td>
                <td>
                  <CardStatusPill state={row.status.state} />
                  {row.status.error && (
                    <InlineMessage tone="warning">
                      {row.status.error}
                    </InlineMessage>
                  )}
                </td>
                <td className="table-actions">
                  <CardRetryButton
                    status={row.status}
                    disabled={busy}
                    onClick={() =>
                      void perform(async () => {
                        await retryStatusPageCard(row.id);
                        await cleanups.refresh();
                      })
                    }
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </section>
  );
}
