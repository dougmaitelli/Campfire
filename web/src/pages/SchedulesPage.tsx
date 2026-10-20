import i18n from "../i18n";
import { useTranslation } from "react-i18next";
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
import { operationLabel } from "../components/servers/operationMessage";
import { useResourceSelection } from "../routing/useResourceSelection";

function when(value: number) {
  return value ? new Date(value * 1000).toLocaleString() : "—";
}
export function SchedulesPage() {
  const { t } = useTranslation();
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
              : i18n.t("schedulesPage.unableToLoadExecutions"),
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
        cause instanceof Error
          ? cause.message
          : i18n.t("schedulesPage.unableToChangeSchedule"),
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
        cause instanceof Error
          ? cause.message
          : i18n.t("schedulesPage.unableToLoadHistory"),
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.schedules")}
        description={t(
          "schedulesPage.oneOffAndRecurringOperationsWithExecutionHistory",
        )}
        actions={
          <>
            <Button
              onClick={() => {
                void perform(async () => {});
              }}
            >
              {t("common.refresh")}
            </Button>
            <Button
              variant="primary"
              disabled={!managed.length}
              onClick={() => {
                setEditing(null);
                setOpen(true);
              }}
            >
              {t("schedulesPage.createSchedule")}
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
        <FormField label={t("common.server")}>
          <Select
            id="schedule-filter"
            value={filter}
            onValueChange={(selectedValue) => {
              setFilter(selectedValue);
              setLoading(true);
              setPolling(true);
            }}
          >
            <option value="">{t("schedulesPage.allAccessibleServers")}</option>
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
          resource={t("schedulesPage.schedules")}
          emptyMessage={
            filter
              ? t("schedulesPage.noMatchingSchedules")
              : t("schedulesPage.noSchedulesYet")
          }
          onRetry={() => {
            void schedules.refresh();
          }}
        >
          <caption className="visually-hidden">{t("common.schedules")}</caption>
          <thead>
            <tr>
              <th>{t("schedulesPage.nameServer")}</th>
              <th>{t("schedulesPage.action")}</th>
              <th>{t("schedulesPage.schedule")}</th>
              <th>{t("schedulesPage.nextRun")}</th>
              <th>{t("schedulesPage.runsAs")}</th>
              <th className="table-actions">{t("common.actions")}</th>
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
                        aria-label={i18n.t("schedulesPage.editSchedule", {
                          itemName: item.name,
                        })}
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
                  <td>{operationLabel(item.action)}</td>
                  <td>
                    <code>
                      {item.cron_expression || i18n.t("schedulesPage.runOnce")}
                    </code>
                    <span className="row-description">{item.time_zone}</span>
                    {item.only_when_running && (
                      <span className="row-description">
                        {i18n.t("schedulesPage.onlyWhileRunning")}
                      </span>
                    )}
                  </td>
                  <td>
                    {item.enabled
                      ? when(item.next_run_at)
                      : i18n.t("schedulesPage.inactive")}
                  </td>
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
                          {i18n.t("schedulesPage.runNow")}
                        </Button>
                      </ActionGroup>
                    )}
                  </td>
                </TableRow>
              ))}
          </tbody>
        </Table>
      </ContentPanel>
      <h2>{t("schedulesPage.executionHistory")}</h2>
      <p className="field-help">
        {t(
          "schedulesPage.includesManualServerOperationsInterruptedWorkMayAlreadyHaveExecuted",
        )}
      </p>
      <ContentPanel>
        <Table
          empty={runs.length === 0}
          loading={loading}
          error={loadError}
          resource={t("schedulesPage.executions")}
          emptyMessage={t("schedulesPage.noExecutionsRecorded")}
          onRetry={() => {
            setLoading(true);
            setPolling(true);
            setRevision((value) => value + 1);
          }}
        >
          <caption className="visually-hidden">
            {t("schedulesPage.executionHistory")}
          </caption>
          <thead>
            <tr>
              <th>{t("schedulesPage.created")}</th>
              <th>{t("schedulesPage.serverAction")}</th>
              <th>{t("schedulesPage.actor")}</th>
              <th>{t("schedulesPage.state")}</th>
              <th>{t("schedulesPage.finished")}</th>
              <th className="table-actions">{t("common.actions")}</th>
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
                    {operationLabel(run.action)} ·{" "}
                    <ResourceLink
                      href={
                        run.schedule_id &&
                        managed.some((server) => server.id === run.server_id)
                          ? links.schedule(run.schedule_id)
                          : undefined
                      }
                    >
                      {run.recurring
                        ? i18n.t("schedulesPage.recurring")
                        : run.schedule_id
                          ? i18n.t("schedulesPage.oneOff")
                          : i18n.t("schedulesPage.manual")}
                    </ResourceLink>
                    {run.retry_of ? i18n.t("schedulesPage.retry") : ""}
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
                              i18n.t(
                                "schedulesPage.retryThisOperationVerifyTheCurrentServerStateFirstAn",
                              ),
                            )
                          )
                            void perform(() => retryExecution(run.id));
                        }}
                      >
                        {i18n.t("schedulesPage.retry2")}
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
            {t("schedulesPage.loadOlderExecutions")}
          </Button>
        )}
      </ContentPanel>
      {!schedules.loading && !schedules.error && missing && (
        <InlineMessage>
          {t("schedulesPage.thisScheduleIsUnavailableOrNoLongerExists")}
        </InlineMessage>
      )}
      {editing &&
        !managed.some((server) => server.id === editing.server_id) && (
          <InlineMessage>
            {t("schedulesPage.youDoNotHavePermissionToEditThisSchedule")}
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
