import { useEffect, useRef, useState } from "react";
import {
  listNotifications,
  markNotificationRead,
  markAllNotificationsRead,
  type Notification,
} from "../api/notifications";
import { useNotifications } from "../hooks/useNotifications";
import "./management.css";

export function NotificationsPage() {
  const inbox = useNotifications();
  const [items, setItems] = useState<Notification[]>([]);
  const [older, setOlder] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [version, setVersion] = useState(0);
  const [paged, setPaged] = useState(false);
  const pending = useRef(false);
  const generation = useRef(0);
  useEffect(() => {
    if (paged) return;
    const controller = new AbortController();
    const current = ++generation.current;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      try {
        const next = await listNotifications(
          AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
        );
        if (!controller.signal.aborted && current === generation.current) {
          setItems(next);
          setOlder(next.length === 100);
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
  }, [version, paged]);

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
      <div className="management-content">
        {loading && <p role="status">Loading notifications…</p>}
        {!loading && !items.length && <p>No notifications.</p>}
        {items.map((item) => (
          <article
            className="notification-message"
            key={item.id}
            aria-label={item.title}
          >
            <div className="notification-content">
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
            </div>
            <button
              className="button"
              aria-label={`${item.read_at ? "Mark unread" : "Mark read"}: ${item.title}`}
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
