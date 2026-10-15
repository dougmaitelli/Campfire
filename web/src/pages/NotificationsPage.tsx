import { useEffect, useRef, useState } from "react";
import {
  listNotifications,
  markNotificationRead,
  markAllNotificationsRead,
  notificationStatus,
  testNotification,
  retryNotificationDeliveries,
  type Notification,
  type NotificationStatus,
} from "../api/notifications";
import { useNotifications } from "../hooks/useNotifications";
import { useAuthentication } from "../hooks/useAuthentication";
import "./management.css";

export function NotificationsPage() {
  const auth = useAuthentication();
  const inbox = useNotifications();
  const [items, setItems] = useState<Notification[]>([]);
  const [status, setStatus] = useState<NotificationStatus | null>(null);
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [older, setOlder] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [version, setVersion] = useState(0);
  const [paged, setPaged] = useState(false);
  const pending = useRef(false);
  const generation = useRef(0);
  const admin = auth.session?.admin === true;
  useEffect(() => {
    if (paged) return;
    const controller = new AbortController();
    const current = ++generation.current;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      try {
        const next = await listNotifications(
          AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
          0,
          unreadOnly,
        );
        const delivery = admin
          ? await notificationStatus(
              AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
            )
          : null;
        if (!controller.signal.aborted && current === generation.current) {
          setItems(next);
          setOlder(next.length === 100);
          setStatus(delivery);
          setError(null);
        }
      } catch (cause) {
        if (!controller.signal.aborted && current === generation.current)
          setError(
            cause instanceof Error
              ? cause.message
              : "Unable to load notifications.",
          );
      } finally {
        if (!controller.signal.aborted && current === generation.current)
          setLoading(false);
      }
      if (!controller.signal.aborted && !paged)
        timer = setTimeout(() => {
          void load();
        }, 15000);
    }
    void load();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [unreadOnly, version, admin, paged]);

  function refresh() {
    setPaged(false);
    setVersion((value) => value + 1);
    inbox.refresh();
  }
  async function perform(action: () => Promise<void>) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await action();
      refresh();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to update notifications.",
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
    setPaged(true);
    const current = ++generation.current;
    try {
      const next = await listNotifications(
        AbortSignal.timeout(15000),
        items.at(-1)?.id,
        unreadOnly,
      );
      if (current === generation.current) {
        setItems((value) => [...value, ...next]);
        setOlder(next.length === 100);
      }
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to load older notifications.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Notifications</h1>
          <p className="page-description">
            Health and resource alerts, recoveries, and messages for your
            account.
          </p>
        </div>
        <div className="page-actions">
          <button className="button" disabled={busy} onClick={refresh}>
            Refresh
          </button>
          <button
            className="button"
            disabled={busy || !items.length}
            onClick={() => {
              void perform(() =>
                markAllNotificationsRead(
                  Math.max(...items.map((item) => item.id)),
                ),
              );
            }}
          >
            Mark all read
          </button>
        </div>
      </header>
      {error && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      <div className="management-content">
        {admin && status && (
          <section aria-label="Notification delivery">
            <h2>Monitoring and delivery</h2>
            <p>
              Apprise:{" "}
              {status.apprise_enabled
                ? "Enabled"
                : "Disabled — in-app notifications remain enabled"}
              . {status.pending} queued · {status.failed} failed.
            </p>
            <p>
              Resource thresholds: CPU{" "}
              {status.cpu_percent === 0 ? "disabled" : `${status.cpu_percent}%`}
              , memory{" "}
              {status.memory_percent === 0
                ? "disabled"
                : `${status.memory_percent}%`}
              , disk{" "}
              {status.disk_percent === 0
                ? "disabled"
                : `${status.disk_percent}%`}
              . Conditions must persist for {status.duration_seconds} seconds.
              Health alerts {status.health_enabled ? "enabled" : "disabled"}.
            </p>
            <div className="page-actions">
              <button
                className="button"
                disabled={busy}
                onClick={() => {
                  void perform(async () => {
                    await testNotification();
                    setNotice(
                      status.apprise_enabled
                        ? "Test notification created and queued for Apprise delivery. Delivery status updates here."
                        : "Test notification created in your inbox.",
                    );
                  });
                }}
              >
                Send test
              </button>
              <button
                className="button"
                disabled={
                  busy || !status.apprise_enabled || status.failed === 0
                }
                onClick={() => {
                  void perform(retryNotificationDeliveries);
                }}
              >
                Retry failed deliveries
              </button>
            </div>
          </section>
        )}
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={unreadOnly}
            disabled={busy}
            onChange={(event) => {
              setUnreadOnly(event.target.checked);
              setPaged(false);
            }}
          />
          Unread only
        </label>
        {loading && <p role="status">Loading notifications…</p>}
        {!loading && !items.length && (
          <p>No {unreadOnly ? "unread " : ""}notifications.</p>
        )}
        {items.map((item) => (
          <article
            className="notification-message"
            key={item.id}
            aria-label={item.title}
          >
            <h2>
              {!item.read_at && <span className="type-badge">Unread</span>}{" "}
              {item.title}
            </h2>
            <p>{item.body}</p>
            <p className="field-help">
              <time dateTime={new Date(item.created_at * 1000).toISOString()}>
                {new Date(item.created_at * 1000).toLocaleString()}
              </time>{" "}
              · {item.severity} · {item.kind}
            </p>
            <button
              className="button"
              disabled={busy}
              onClick={() => {
                void perform(() =>
                  markNotificationRead(item.id, !item.read_at),
                );
              }}
            >
              {item.read_at ? "Mark unread" : "Mark read"}
            </button>
          </article>
        ))}
        {older && (
          <button
            className="button"
            disabled={busy}
            onClick={() => {
              void more();
            }}
          >
            Load older
          </button>
        )}
      </div>
    </section>
  );
}
