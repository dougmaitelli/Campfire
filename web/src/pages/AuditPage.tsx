import { useEffect, useState } from "react";
import { activityCursor, listAudit, type AuditEvent } from "../api/activity";
import "./management.css";

export function AuditPage() {
  const [items, setItems] = useState<AuditEvent[]>([]);
  const [action, setAction] = useState("");
  const [target, setTarget] = useState("");
  const [filters, setFilters] = useState({ action: "", target: "" });
  const [version, setVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  const [older, setOlder] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    void listAudit(controller.signal, "", filters.action, filters.target)
      .then((next) => {
        if (!controller.signal.aborted) {
          setItems(next);
          setOlder(next.length === 100);
          setError(null);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : "Unable to load audit records.",
          );
      });
    return () => controller.abort();
  }, [filters, version]);
  async function more() {
    if (busy || !items.length) return;
    setBusy(true);
    try {
      const next = await listAudit(
        AbortSignal.timeout(15000),
        activityCursor(items.at(-1)),
        filters.action,
        filters.target,
      );
      setItems((current) => [...current, ...next]);
      setOlder(next.length === 100);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to load audit records.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Audit</h1>
          <p className="page-description">
            Administrative changes, authentication attempts, and server
            operations.
          </p>
        </div>
        <button className="button" onClick={() => setVersion((v) => v + 1)}>
          Refresh
        </button>
      </header>
      {error && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
      <div className="management-content">
        <form
          className="page-actions"
          onSubmit={(e) => {
            e.preventDefault();
            setFilters({ action: action.trim(), target: target.trim() });
          }}
        >
          <label>
            Action
            <input
              value={action}
              maxLength={100}
              placeholder="server.restart"
              onChange={(e) => setAction(e.target.value)}
            />
          </label>
          <label>
            Target
            <input
              value={target}
              maxLength={150}
              placeholder="server:ID"
              onChange={(e) => setTarget(e.target.value)}
            />
          </label>
          <button className="button">Filter</button>
        </form>
        <div className="table-container">
          <table className="management-table">
            <caption className="visually-hidden">Audit records</caption>
            <thead>
              <tr>
                <th>Time</th>
                <th>Actor</th>
                <th>Action</th>
                <th>Target</th>
                <th>Outcome</th>
                <th>HTTP status</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr key={item.id}>
                  <td>{new Date(item.created_at * 1000).toLocaleString()}</td>
                  <td>
                    {item.username || "Unauthenticated"}
                    {item.api_token_id
                      ? ` (API token #${item.api_token_id})`
                      : ""}
                  </td>
                  <td>{item.action}</td>
                  <td>{item.target}</td>
                  <td>{item.outcome}</td>
                  <td>{item.status || "—"}</td>
                </tr>
              ))}
              {!items.length && (
                <tr>
                  <td colSpan={6}>No audit records.</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
        {older && (
          <button
            className="button"
            disabled={busy}
            onClick={() => {
              void more();
            }}
          >
            Load older records
          </button>
        )}
      </div>
    </section>
  );
}
