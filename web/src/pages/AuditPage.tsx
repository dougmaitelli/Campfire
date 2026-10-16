import { useEffect, useState } from "react";
import { activityCursor, listAudit, type AuditEvent } from "../api/activity";
import "./management.css";
import { Table } from "../components/table/Table";
import { StatusPill } from "../components/status/StatusPill";

export function AuditPage() {
  const [items, setItems] = useState<AuditEvent[]>([]);
  const [action, setAction] = useState("");
  const [target, setTarget] = useState("");
  const [filters, setFilters] = useState({ action: "", target: "" });
  const [version, setVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  const [older, setOlder] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
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
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
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
        <button
          className="button"
          onClick={() => {
            setLoading(true);
            setVersion((v) => v + 1);
          }}
        >
          Refresh
        </button>
      </header>
      {error && items.length > 0 && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
      <form
        className="list-toolbar"
        onSubmit={(e) => {
          e.preventDefault();
          setLoading(true);
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
      <div className="management-content">
        <Table
          empty={items.length === 0}
          loading={loading}
          error={error}
          resource="audit records"
          emptyMessage="No audit records"
          onRetry={() => {
            setLoading(true);
            setVersion((value) => value + 1);
          }}
        >
          <caption className="visually-hidden">Audit records</caption>
          <thead>
            <tr>
              <th>Time</th>
              <th>Actor</th>
              <th>Action</th>
              <th>Target</th>
              <th>Outcome</th>
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
                <td>
                  <StatusPill status={item.outcome} />
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
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
