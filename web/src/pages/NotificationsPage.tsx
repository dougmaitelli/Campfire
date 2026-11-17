import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { ContentState } from "../components/layout/ContentState";
import { PageHeader } from "../components/layout/PageHeader";
import { Pagination } from "../components/table/Pagination";
import { usePagination } from "../hooks/usePagination";
import { usePagedCollection } from "../hooks/usePagedCollection";
import {
  listNotifications,
  NOTIFICATIONS_REFRESH_INTERVAL_MS,
  markNotificationRead,
  markAllNotificationsRead,
} from "../api/notifications";
import { useNotifications } from "../hooks/useNotifications";
import { API_READ_TIMEOUT_MS } from "../api/client";
import { StatusPill } from "../components/status/StatusPill";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import "./notifications.css";

export function NotificationsPage() {
  const { t } = useTranslation();
  const severityLabels: Record<string, string> = {
    info: t("notificationsPage.severityInfo"),
    warning: t("notificationsPage.severityWarning"),
    critical: t("notificationsPage.severityCritical"),
    error: t("notificationsPage.severityError"),
  };
  const links = useResourceLinks();
  const inbox = useNotifications();
  const navigation = usePagination();
  const load = useCallback(
    (signal: AbortSignal, page: number) =>
      listNotifications({ signal, page, pageSize: navigation.pageSize }),
    [navigation.pageSize],
  );
  const history = usePagedCollection(
    load,
    navigation.page,
    NOTIFICATIONS_REFRESH_INTERVAL_MS,
  );
  const { items, loading } = history;
  const [busy, setBusy] = useState(false);
  const [actionError, setError] = useState<string | null>(null);
  const error = actionError ?? history.error;
  const pending = useRef(false);
  async function perform(action: () => Promise<void>) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await action();
      await history.refresh();
      inbox.refresh();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : t("notificationsPage.unableToUpdateNotifications"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.notifications")}
        description={t(
          "notificationsPage.healthAndResourceAlertsRecoveriesAndMessagesForYourAccount",
        )}
        actions={
          <Button
            disabled={busy || !items.length}
            onClick={() =>
              void perform(async () => {
                // Later pages do not contain the inbox's newest watermark.
                // Read it without replacing the displayed page, and never send
                // zero (which would also acknowledge newly arriving messages).
                const newest =
                  navigation.page === 1
                    ? items
                    : (
                        await listNotifications({
                          signal: AbortSignal.timeout(API_READ_TIMEOUT_MS),
                          page: 1,
                        })
                      ).items;
                if (newest.length)
                  await markAllNotificationsRead(
                    Math.max(...newest.map((item) => item.id)),
                  );
              })
            }
          >
            {t("notificationsPage.markAllRead")}
          </Button>
        }
      />
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
      <ContentPanel>
        {(loading || !items.length) && (
          <ContentState
            loading={loading}
            error={error}
            resource={t("notificationsPage.notifications")}
            emptyMessage={t("notificationsPage.noNotifications")}
            onRetry={() => void history.refresh()}
          />
        )}
        {items.map((item) => (
          <article
            className="notification-message"
            key={item.id}
            aria-label={item.title}
          >
            <div className="notification-content">
              <h2>
                <ResourceLink
                  href={links.target(`${item.target_type}:${item.target_id}`)}
                >
                  {item.title}
                </ResourceLink>
                {!item.read_at && (
                  <StatusPill tone="warning">
                    {t("notificationsPage.unread")}
                  </StatusPill>
                )}
              </h2>
              <p>{item.body}</p>
              <p className="field-help">
                <time dateTime={new Date(item.created_at * 1000).toISOString()}>
                  {new Date(item.created_at * 1000).toLocaleString()}
                </time>{" "}
                · {severityLabels[item.severity] ?? item.severity} · {item.kind}
              </p>
            </div>
            <Button
              size="small"
              iconOnly
              variant={item.read_at ? "default" : "primary"}
              title={t(
                item.read_at
                  ? "notificationsPage.markUnread"
                  : "notificationsPage.markRead",
              )}
              aria-label={t(
                item.read_at
                  ? "notificationsPage.markUnreadTitle"
                  : "notificationsPage.markReadTitle",
                { title: item.title },
              )}
              disabled={busy}
              onClick={() =>
                void perform(() => markNotificationRead(item.id, !item.read_at))
              }
            >
              <NavigationIcon name={item.read_at ? "mail-open" : "mail"} />
            </Button>
          </article>
        ))}
        <Pagination
          pagination={history.pagination}
          onPageChange={navigation.setPage}
          onPageSizeChange={navigation.setPageSize}
          loading={loading || busy}
        />
      </ContentPanel>
    </section>
  );
}
