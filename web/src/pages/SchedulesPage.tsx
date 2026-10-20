import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { ActionGroup } from "../components/layout/ActionGroup";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { ListToolbar } from "../components/layout/ListToolbar";
import { PageHeader } from "../components/layout/PageHeader";
import { Select } from "../components/forms/Select";
import { useEffect, useRef, useState } from "react";
import { listServers } from "../api/servers";
import {
  activityCursor,
  listExecutions,
  listSchedules,
  retryExecution,
  runSchedule,
  type Execution,
} from "../api/activity";
import { useCollection } from "../hooks/useCollection";
import { ScheduleDialog } from "../dialogs/ScheduleDialog";

import { Table, TableRow } from "../components/table/Table";
import { ExecutionState } from "../components/schedules/ExecutionState";
import { useResourceSelection } from "../routing/useResourceSelection";

function when(value: number) {
  return value ? new Date(value * 1000).toLocaleString() : "—";
}
export function SchedulesPage() {
  const links = useResourceLinks();
  const servers = useCollection(listServers);
  const schedules = useCollection(listSchedules);
  const [runs, setRuns] = useState<Execution[]>([]);
  const {
    selected: editing,
    select: setEditing,
    missing,
  } = useResourceSelection(schedules.items);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [filter, setFilter] = useState("");
  const [loading, setLoading] = useState(true);
  const [older, setOlder] = useState(false);
  const [polling, setPolling] = useState(true);
  const [revision, setRevision] = useState(0);
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
          setLoadError(null);
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setLoadError(
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
  }, [filter, polling, replaceSchedules, revision]);
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
        activityCursor(runs.at(-1)),
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
      <PageHeader
        title="Schedules"
        description="One-off and recurring operations with execution history."
        actions={
          <>
            <Button
              onClick={() => {
                void perform(async () => {});
              }}
            >
              Refresh
            </Button>
            <Button
              variant="primary"
              disabled={!managed.length}
              onClick={() => {
                setEditing(null);
                setOpen(true);
              }}
            >
              + Create schedule
            </Button>
          </>
        }
      />
      {(error ??
        (schedules.items.length ? schedules.error : null) ??
        (servers.items.length ? servers.error : null)) && (
        <InlineMessage role="alert">
          {error ?? schedules.error ?? servers.error}
        </InlineMessage>
      )}
      <ListToolbar>
        <FormField label="Server">
          <Select
            id="schedule-filter"
            value={filter}
            onValueChange={(selectedValue) => {
              setFilter(selectedValue);
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
          </Select>
        </FormField>
      </ListToolbar>
      <ContentPanel>
        <Table
          empty={
            !schedules.items.some(
              (item) => !filter || item.server_id === filter,
            )
          }
          loading={schedules.loading}
          error={schedules.error}
          resource="schedules"
          emptyMessage={filter ? "No matching schedules" : "No schedules yet"}
          onRetry={() => {
            void schedules.refresh();
          }}
        >
          <caption className="visually-hidden">Schedules</caption>
          <thead>
            <tr>
              <th>Name / server</th>
              <th>Action</th>
              <th>Schedule</th>
              <th>Next run</th>
              <th>Runs as</th>
              <th className="table-actions">Actions</th>
            </tr>
          </thead>
          <tbody>
            {schedules.items
              .filter((item) => !filter || item.server_id === filter)
              .map((item) => (
                <TableRow
                  key={item.id}
                  onOpen={
                    !busy &&
                    managed.some((server) => server.id === item.server_id)
                      ? () => {
                          setEditing(item);
                          setOpen(false);
                        }
                      : undefined
                  }
                >
                  <td>
                    {managed.some((server) => server.id === item.server_id) ? (
                      <button
                        type="button"
                        className="table-row-trigger"
                        disabled={busy}
                        aria-label={`Edit schedule ${item.name}`}
                        onClick={() => {
                          setEditing(item);
                          setOpen(false);
                        }}
                      >
                        {item.name}
                      </button>
                    ) : (
                      <strong>{item.name}</strong>
                    )}
                    <span className="row-description">
                      <ResourceLink href={links.server(item.server_id)}>
                        {servers.items.find(
                          (server) => server.id === item.server_id,
                        )?.name ?? item.server_id}
                      </ResourceLink>
                    </span>
                  </td>
                  <td>{item.action}</td>
                  <td>
                    <code>{item.cron_expression || "Run once"}</code>
                    <span className="row-description">{item.time_zone}</span>
                    {item.only_when_running && (
                      <span className="row-description">
                        Only while running
                      </span>
                    )}
                  </td>
                  <td>{item.enabled ? when(item.next_run_at) : "Inactive"}</td>
                  <td>
                    <ResourceLink href={links.user(item.run_as_user_id)}>
                      {item.username}
                    </ResourceLink>
                  </td>
                  <td className="table-actions">
                    {managed.some((server) => server.id === item.server_id) && (
                      <ActionGroup>
                        <Button
                          disabled={busy}
                          onClick={() => {
                            void perform(() => runSchedule(item.id));
                          }}
                        >
                          Run now
                        </Button>
                      </ActionGroup>
                    )}
                  </td>
                </TableRow>
              ))}
          </tbody>
        </Table>
      </ContentPanel>
      <h2>Execution history</h2>
      <p className="field-help">
        Includes manual server operations. Interrupted work may already have
        executed; inspect the server before retrying.
      </p>
      <ContentPanel>
        <Table
          empty={runs.length === 0}
          loading={loading}
          error={loadError}
          resource="executions"
          emptyMessage="No executions recorded"
          onRetry={() => {
            setLoading(true);
            setPolling(true);
            setRevision((value) => value + 1);
          }}
        >
          <caption className="visually-hidden">Execution history</caption>
          <thead>
            <tr>
              <th>Created</th>
              <th>Server / action</th>
              <th>Actor</th>
              <th>State</th>
              <th>Finished</th>
              <th className="table-actions">Actions</th>
            </tr>
          </thead>
          <tbody>
            {runs.map((run) => (
              <tr key={run.id}>
                <td>{when(run.created_at)}</td>
                <td>
                  <ResourceLink href={links.server(run.server_id)}>
                    {run.server_name}
                  </ResourceLink>
                  <span className="row-description">
                    {run.action} ·{" "}
                    <ResourceLink
                      href={
                        run.schedule_id &&
                        managed.some((server) => server.id === run.server_id)
                          ? links.schedule(run.schedule_id)
                          : undefined
                      }
                    >
                      {run.recurring
                        ? "Recurring"
                        : run.schedule_id
                          ? "One-off"
                          : "Manual"}
                    </ResourceLink>
                    {run.retry_of ? " · Retry" : ""}
                  </span>
                </td>
                <td>
                  <ResourceLink href={links.user(run.user_id)}>
                    {run.username}
                  </ResourceLink>
                </td>
                <td>
                  <ExecutionState state={run.state} />
                  <span className="row-description">{run.message}</span>
                </td>
                <td>{when(run.finished_at)}</td>
                <td className="table-actions">
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
                      <Button
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
                      </Button>
                    )}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        {older && (
          <Button
            disabled={busy}
            onClick={() => {
              void more();
            }}
          >
            Load older executions
          </Button>
        )}
      </ContentPanel>
      {!schedules.loading && !schedules.error && missing && (
        <InlineMessage>
          This schedule is unavailable or no longer exists.
        </InlineMessage>
      )}
      {editing &&
        !managed.some((server) => server.id === editing.server_id) && (
          <InlineMessage>
            You do not have permission to edit this schedule.
          </InlineMessage>
        )}
      {(open ||
        (editing &&
          managed.some((server) => server.id === editing.server_id))) && (
        <ScheduleDialog
          key={editing?.id ?? "new"}
          schedule={editing}
          servers={managed}
          onClose={() => {
            setOpen(false);
            setEditing(null);
          }}
          onSaved={() => {
            void schedules.refresh();
          }}
        />
      )}
    </section>
  );
}
