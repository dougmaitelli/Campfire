import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { ContentState } from "../components/layout/ContentState";
import { PageHeader } from "../components/layout/PageHeader";
import { useEffect, useRef, useState } from "react";
import {
  listNotifications,
  NOTIFICATIONS_REFRESH_INTERVAL_MS,
  markNotificationRead,
  markAllNotificationsRead,
  type Notification,
} from "../api/notifications";
import { useNotifications } from "../hooks/useNotifications";
import { StatusPill } from "../components/status/StatusPill";
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
  const [items, setItems] = useState<Notification[]>([]);
  const [older, setOlder] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [version, setVersion] = useState(0);
  const paged = useRef(false);
  const pending = useRef(false);
  const generation = useRef(0);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      if (pending.current) {
        timer = setTimeout(
          () => void load(),
          NOTIFICATIONS_REFRESH_INTERVAL_MS,
        );
        return;
      }
      const current = ++generation.current;
      try {
        const next = await listNotifications(
          AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
        );
        if (!controller.signal.aborted && current === generation.current) {
          setItems((previous) =>
            paged.current
              ? [
                  ...new Map(
                    [...previous, ...next].map((item) => [item.id, item]),
                  ).values(),
                ].sort((a, b) => b.id - a.id)
              : next,
          );
          if (!paged.current) setOlder(next.length === 100);
          setError(null);
        }
      } catch (cause) {
        if (!controller.signal.aborted && current === generation.current)
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("notificationsPage.unableToLoadNotifications"),
          );
      } finally {
        if (!controller.signal.aborted && current === generation.current)
          setLoading(false);
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => {
          void load();
        }, NOTIFICATIONS_REFRESH_INTERVAL_MS);
    }
    void load();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [version]);

  function refresh() {
    paged.current = false;
    setVersion((value) => value + 1);
    inbox.refresh();
  }
  async function perform(action: () => Promise<void>) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await action();
      refresh();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("notificationsPage.unableToUpdateNotifications"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  async function more() {
    if (pending.current || !items.length) return;
    pending.current = true;
    setBusy(true);
    paged.current = true;
    const current = ++generation.current;
    try {
      const next = await listNotifications(
        AbortSignal.timeout(15000),
        items.at(-1)?.id,
      );
      if (current === generation.current) {
        setItems((value) => [...value, ...next]);
        setOlder(next.length === 100);
      }
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("notificationsPage.unableToLoadOlderNotifications"),
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
          <>
            <Button
              disabled={busy || !items.length}
              onClick={() => {
                void perform(() =>
                  markAllNotificationsRead(
                    Math.max(...items.map((item) => item.id)),
                  ),
                );
              }}
            >
              {t("notificationsPage.markAllRead")}
            </Button>
          </>
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
            onRetry={refresh}
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
                    {i18n.t("notificationsPage.unread")}
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
              aria-label={i18n.t(
                item.read_at
                  ? "notificationsPage.markUnreadTitle"
                  : "notificationsPage.markReadTitle",
                { title: item.title },
              )}
              disabled={busy}
              onClick={() => {
                void perform(() =>
                  markNotificationRead(item.id, !item.read_at),
                );
              }}
            >
              {item.read_at
                ? i18n.t("notificationsPage.markUnread")
                : i18n.t("notificationsPage.markRead")}
            </Button>
          </article>
        ))}
        {older && (
          <Button
            disabled={busy}
            onClick={() => {
              void more();
            }}
          >
            {t("notificationsPage.loadOlder")}
          </Button>
        )}
      </ContentPanel>
    </section>
  );
}
