import { useEffect, useRef, useState } from "react";
import { listServers } from "../api/servers";
import {
  deleteSchedule,
  listExecutions,
  listSchedules,
  retryExecution,
  runSchedule,
  type Execution,
  type Schedule,
} from "../api/activity";
import { useCollection } from "../hooks/useCollection";
import { ScheduleDialog } from "../dialogs/ScheduleDialog";
import "./management.css";

function when(value: number) {
  return value ? new Date(value * 1000).toLocaleString() : "—";
}
export function SchedulesPage() {
  const servers = useCollection(listServers);
  const schedules = useCollection(listSchedules);
  const [runs, setRuns] = useState<Execution[]>([]);
  const [editing, setEditing] = useState<Schedule | null>(null);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [filter, setFilter] = useState("");
  const [loading, setLoading] = useState(true);
  const [older, setOlder] = useState(false);
  const [polling, setPolling] = useState(true);
  const generation = useRef(0);
  const managed = servers.items.filter(
    (server) =>
      server.permissions.includes("server.configure") &&
      server.permissions.includes("server.operate"),
  );
  const replaceSchedules = schedules.replace;
  useEffect(() => {
    if (!polling) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const version = ++generation.current;
    async function poll() {
      try {
        const [items, recurring] = await Promise.all([
          listExecutions(controller.signal, "", filter),
          listSchedules(controller.signal),
        ]);
        if (!controller.signal.aborted && version === generation.current) {
          setRuns(items);
          replaceSchedules(recurring);
          setOlder(items.length === 100);
          setError(null);
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : "Unable to load executions.",
          );
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => {
          void poll();
        }, 5000);
    }
    void poll();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [filter, polling, replaceSchedules]);
  async function perform(work: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError(null);
    setPolling(true);
    const version = generation.current;
    try {
      await work();
      await schedules.refresh();
      const items = await listExecutions(
        AbortSignal.timeout(15000),
        "",
        filter,
      );
      if (version === generation.current) {
        setRuns(items);
        setOlder(items.length === 100);
      }
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to change schedule.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function more() {
    if (busy || !runs.length) return;
    const version = ++generation.current;
    setPolling(false);
    setBusy(true);
    try {
      const items = await listExecutions(
        AbortSignal.timeout(15000),
        runs.at(-1)?.id,
        filter,
      );
      if (version === generation.current) {
        setRuns((current) => [...current, ...items]);
        setOlder(items.length === 100);
      }
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to load history.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Schedules</h1>
          <p className="page-description">
            Recurring operations and durable execution history.
          </p>
        </div>
        <div className="page-actions">
          <button
            className="button"
            onClick={() => {
              void perform(async () => {});
            }}
          >
            Refresh
          </button>
          <button
            className="button button-primary"
            disabled={!managed.length}
            onClick={() => {
              setEditing(null);
              setOpen(true);
            }}
          >
            + Create schedule
          </button>
        </div>
      </header>
      {(error ?? schedules.error ?? servers.error) && (
        <p className="error-message" role="alert">
          {error ?? schedules.error ?? servers.error}
        </p>
      )}
      <div className="management-content">
        <label htmlFor="schedule-filter">Server</label>
        <select
          id="schedule-filter"
          value={filter}
          onChange={(e) => {
            setFilter(e.target.value);
            setLoading(true);
            setPolling(true);
          }}
        >
          <option value="">All accessible servers</option>
          {servers.items.map((server) => (
            <option key={server.id} value={server.id}>
              {server.name}
            </option>
          ))}
        </select>
        <h2>Recurring schedules</h2>
        <div className="table-container">
          <table className="management-table">
            <caption className="visually-hidden">Recurring schedules</caption>
            <thead>
              <tr>
                <th>Name / server</th>
                <th>Action</th>
                <th>Interval</th>
                <th>Next run</th>
                <th>Runs as</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {schedules.items
                .filter((item) => !filter || item.server_id === filter)
                .map((item) => (
                  <tr key={item.id}>
                    <td>
                      <strong>{item.name}</strong>
                      <span className="row-description">
                        {servers.items.find(
                          (server) => server.id === item.server_id,
                        )?.name ?? item.server_id}
                      </span>
                    </td>
                    <td>{item.action}</td>
                    <td>{item.interval_minutes} minutes</td>
                    <td>{item.enabled ? when(item.next_run_at) : "Paused"}</td>
                    <td>{item.username}</td>
                    <td>
                      {managed.some(
                        (server) => server.id === item.server_id,
                      ) && (
                        <div className="row-actions">
                          <button
                            className="button"
                            disabled={busy}
                            onClick={() => {
                              void perform(() => runSchedule(item.id));
                            }}
                          >
                            Run now
                          </button>
                          <button
                            className="button"
                            disabled={busy}
                            onClick={() => {
                              setEditing(item);
                              setOpen(true);
                            }}
                          >
                            Edit
                          </button>
                          <button
                            className="button"
                            disabled={busy}
                            onClick={() => {
                              if (
                                window.confirm(
                                  `Delete schedule "${item.name}"? History will be retained.`,
                                )
                              )
                                void perform(() => deleteSchedule(item.id));
                            }}
                          >
                            Delete
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              {!schedules.loading &&
                !schedules.items.some(
                  (item) => !filter || item.server_id === filter,
                ) && (
                  <tr>
                    <td colSpan={6}>No recurring schedules.</td>
                  </tr>
                )}
            </tbody>
          </table>
        </div>
        <h2>Execution history</h2>
        <p className="field-help">
          Includes manual server operations. Interrupted work may already have
          executed; inspect the server before retrying.
        </p>
        <div className="table-container">
          <table className="management-table">
            <caption className="visually-hidden">Execution history</caption>
            <thead>
              <tr>
                <th>Created</th>
                <th>Server / action</th>
                <th>Actor</th>
                <th>State</th>
                <th>Finished</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((run) => (
                <tr key={run.id}>
                  <td>{when(run.created_at)}</td>
                  <td>
                    <strong>{run.server_name}</strong>
                    <span className="row-description">
                      {run.action} · {run.recurring ? "Recurring" : "Manual"}
                      {run.retry_of ? " · Retry" : ""}
                    </span>
                  </td>
                  <td>{run.username}</td>
                  <td>
                    {run.state}
                    <span className="row-description">{run.message}</span>
                  </td>
                  <td>{when(run.finished_at)}</td>
                  <td>
                    {["failed", "interrupted"].includes(run.state) &&
                      [
                        "install",
                        "start",
                        "stop",
                        "restart",
                        "kill",
                        "backup",
                      ].includes(run.action) &&
                      servers.items.some(
                        (server) =>
                          server.id === run.server_id &&
                          server.permissions.includes("server.operate"),
                      ) && (
                        <button
                          className="button"
                          disabled={busy}
                          onClick={() => {
                            if (
                              window.confirm(
                                "Retry this operation? Verify the current server state first; an interrupted operation may already have executed.",
                              )
                            )
                              void perform(() => retryExecution(run.id));
                          }}
                        >
                          Retry
                        </button>
                      )}
                  </td>
                </tr>
              ))}
              {!runs.length && (
                <tr>
                  <td colSpan={6}>
                    {loading ? "Loading…" : "No executions recorded."}
                  </td>
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
            Load older executions
          </button>
        )}
      </div>
      {open && (
        <ScheduleDialog
          schedule={editing}
          servers={managed}
          onClose={() => setOpen(false)}
          onSaved={() => {
            void schedules.refresh();
          }}
        />
      )}
    </section>
  );
}
