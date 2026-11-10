import { useTranslation } from "react-i18next";
import type { ActivityEntry } from "../../api/serverActivity";
import { Table, TableRow } from "../table/Table";
import { ActivityStatus, activityTitle } from "./ActivityStatus";

export function ActivityTable({
  items,
  loading,
  error,
  onOpen,
  onRetry,
  notifications = false,
}: {
  items: ActivityEntry[];
  loading?: boolean;
  error?: string | null;
  onOpen: (key: string) => void;
  onRetry?: (() => void) | undefined;
  notifications?: boolean;
}) {
  const { t, i18n } = useTranslation();
  return (
    <Table
      aria-label={t(
        notifications ? "serverActivity.notifications" : "serverActivity.title",
      )}
      className="activity-table table-cards"
      empty={!items.length}
      loading={loading ?? false}
      error={error}
      resource={t("serverActivity.title")}
      emptyMessage={t("serverActivity.empty")}
      emptyIcon="jobs"
      onRetry={onRetry}
    >
      <thead>
        <tr>
          <th>{t("serverActivity.recorded")}</th>
          <th>{t("serverActivity.type")}</th>
          <th>{t("serverActivity.summary")}</th>
          <th>{t("common.status")}</th>
          <th>
            {t(
              notifications
                ? "serverActivity.destination"
                : "serverActivity.notifications",
            )}
          </th>
        </tr>
      </thead>
      <tbody>
        {items.map((item) => (
          <TableRow
            key={item.key}
            onOpen={() => onOpen(item.key)}
            aria-label={
              item.kind === "job"
                ? t("serverJobs.job", {
                    type: activityTitle(item),
                    id: item.key.slice(4),
                  })
                : undefined
            }
          >
            <td data-label={t("serverActivity.recorded")}>
              <time dateTime={new Date(item.created_at * 1000).toISOString()}>
                {new Date(item.created_at * 1000).toLocaleString(i18n.language)}
              </time>
            </td>
            <td data-label={t("serverActivity.type")}>
              {t(`serverActivity.kinds.${item.kind}`)}
            </td>
            <td data-label={t("serverActivity.summary")}>
              <button
                type="button"
                className="table-row-trigger"
                onClick={() => onOpen(item.key)}
              >
                {activityTitle(item)}
              </button>
              {item.kind === "notification" && (
                <span className="row-description">
                  {item.attachment_name
                    ? t(
                        item.attachment_content_type?.startsWith("image/")
                          ? "serverActivity.imageMessage"
                          : item.attachment_content_type?.startsWith("video/")
                            ? "serverActivity.videoMessage"
                            : "serverActivity.attachmentMessage",
                        { name: item.attachment_name },
                      )
                    : t("serverActivity.textMessage")}
                </span>
              )}
            </td>
            <td data-label={t("common.status")}>
              <ActivityStatus state={item.state} />
            </td>
            <td
              data-label={t(
                notifications
                  ? "serverActivity.destination"
                  : "serverActivity.notifications",
              )}
            >
              {item.kind === "notification" ? (
                <>
                  <span>{item.provider}</span>
                  {item.destination && (
                    <span className="activity-destination">
                      #{item.destination}
                    </span>
                  )}
                </>
              ) : (
                item.notification_count || "—"
              )}
            </td>
          </TableRow>
        ))}
      </tbody>
    </Table>
  );
}
