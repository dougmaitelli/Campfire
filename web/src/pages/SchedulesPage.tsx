import {
  ServerAction,
  ExecutionState as ExecutionStatus,
} from "../contracts/domains";
import i18n from "../i18n";
import {
  canPerformServer,
  canManageSchedule,
  canRunSchedule,
  canRetryExecution,
} from "../permissions";
import { useTranslation } from "react-i18next";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
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
import { useCallback, useRef, useState } from "react";
import { usePagination } from "../hooks/usePagination";
import { usePagedCollection } from "../hooks/usePagedCollection";
import { Pagination } from "../components/table/Pagination";
import { listServers } from "../api/servers";
import {
  listExecutions,
  listSchedules,
  retryExecution,
  runSchedule,
  type Execution,
} from "../api/activity";
import { useCollection } from "../hooks/useCollection";
import { ScheduleDrawer } from "../drawers/ScheduleDrawer";
import { ScheduleDetailsDrawer } from "../drawers/ScheduleDetailsDrawer";

import { Table, TableRow } from "../components/table/Table";
import { ExecutionState } from "../components/schedules/ExecutionState";
import { ScheduleToggle } from "../components/schedules/ScheduleToggle";
import { ScheduleNextRun } from "../components/schedules/ScheduleNextRun";
import { operationLabel } from "../components/servers/operationMessage";
import { useResourceSelection } from "../routing/useResourceSelection";
import { useConfirmation } from "../hooks/useConfirmation";

function when(value: number) {
  return value ? new Date(value * 1000).toLocaleString() : "—";
}

export function SchedulesPage() {
  const { t } = useTranslation();
  const confirmation = useConfirmation();
  const runNowLabel = t("schedulesPage.runNow");
  const retryLabel = t("schedulesPage.retry2");
  const links = useResourceLinks();
  const servers = useCollection(listServers);
  const schedules = useCollection(listSchedules, 10000);
  const { selected: details, select: setDetails } = useResourceSelection(
    schedules.items,
    !schedules.loading && !schedules.error,
  );
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const navigation = usePagination();
  const filter = navigation.query.get("server_id") ?? "";
  const setFilter = (value: string) =>
    navigation.update({ server_id: value, page: null });
  const load = useCallback(
    (signal: AbortSignal, page: number) => listExecutions(signal, page, filter),
    [filter],
  );
  const history = usePagedCollection(load, navigation.page);
  const { items: runs, loading, error: loadError } = history;
  const pending = useRef(false);
  const managed = servers.items.filter((server) =>
    canPerformServer(server, "manageSchedules"),
  );
  async function perform(work: () => Promise<unknown>) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await work();
      await schedules.refresh();
      await history.refresh();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("schedulesPage.unableToChangeSchedule"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  async function retry(run: Execution) {
    if (
      pending.current ||
      !(await confirmation.confirm({
        title: t("confirmDialog.retryOperation"),
        description: t(
          "schedulesPage.retryThisOperationVerifyTheCurrentServerStateFirstAn",
        ),
        confirmLabel: retryLabel,
        warning: true,
      }))
    )
      return;
    await perform(() => retryExecution(run.id));
  }

  return (
    <section aria-labelledby="page-title">
      {confirmation.dialog}
      <PageHeader
        title={t("common.schedules")}
        description={t(
          "schedulesPage.oneOffAndRecurringOperationsWithExecutionHistory",
        )}
        primaryAction={{
          label: t("schedulesPage.createSchedule"),
          compactLabel: t("common.createAction"),
          disabled: !managed.length,
          onClick: () => {
            setDetails(null);
            setOpen(true);
          },
        }}
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
              <th className="table-mobile-hidden">
                {t("schedulesPage.action")}
              </th>
              <th className="table-mobile-hidden">
                {t("schedulesPage.schedule")}
              </th>
              <th>{t("schedulesPage.nextRun")}</th>
              <th className="table-mobile-hidden">
                {t("schedulesPage.runsAs")}
              </th>
              <th>{t("common.enabled")}</th>
              <th className="table-actions">
                <span className="visually-hidden">{t("common.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {schedules.items
              .filter((item) => !filter || item.server_id === filter)
              .map((item) => (
                <TableRow
                  key={item.id}
                  onOpen={
                    !busy
                      ? () => {
                          setDetails(item);
                          setOpen(false);
                        }
                      : undefined
                  }
                >
                  <td>
                    <button
                      type="button"
                      className="table-row-trigger"
                      disabled={busy}
                      aria-label={i18n.t("schedulesPage.viewDetailsFor", {
                        itemName: item.name,
                      })}
                      onClick={() => {
                        setDetails(item);
                        setOpen(false);
                      }}
                    >
                      {item.name}
                    </button>
                    <span className="row-description">
                      <ResourceLink href={links.server(item.server_id)}>
                        {servers.items.find(
                          (server) => server.id === item.server_id,
                        )?.name ?? item.server_id}
                      </ResourceLink>
                    </span>
                  </td>
                  <td className="table-mobile-hidden">
                    {operationLabel(item.action)}
                  </td>
                  <td className="table-mobile-hidden">
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
                    <ScheduleNextRun
                      at={item.next_run_at}
                      enabled={item.enabled}
                    />
                  </td>
                  <td className="table-mobile-hidden">
                    <ResourceLink href={links.user(item.run_as_user_id)}>
                      {item.username}
                    </ResourceLink>
                  </td>
                  <td>
                    <ScheduleToggle
                      schedule={item}
                      disabled={
                        busy ||
                        !managed.some(
                          (server) =>
                            server.id === item.server_id &&
                            canManageSchedule(server, item.action),
                        )
                      }
                      onSaved={schedules.upsert}
                      onError={setError}
                    />
                  </td>
                  <td className="table-actions">
                    {servers.items.some(
                      (server) =>
                        server.id === item.server_id &&
                        canRunSchedule(server, item.action),
                    ) && (
                      <ActionGroup>
                        <Button
                          iconOnly
                          aria-label={runNowLabel}
                          title={runNowLabel}
                          disabled={busy}
                          onClick={() => {
                            void perform(() => runSchedule(item.id));
                          }}
                        >
                          <NavigationIcon name="play" filled />
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
        {t("schedulesPage.scheduleHistoryDescription")}
      </p>
      <ContentPanel>
        <Table
          empty={runs.length === 0}
          loading={loading}
          error={loadError}
          resource={t("schedulesPage.executions")}
          emptyMessage={t("schedulesPage.noExecutionsRecorded")}
          onRetry={() => void history.refresh()}
        >
          <caption className="visually-hidden">
            {t("schedulesPage.executionHistory")}
          </caption>
          <thead>
            <tr>
              <th>{t("schedulesPage.created")}</th>
              <th>{t("schedulesPage.serverAction")}</th>
              <th className="table-mobile-hidden">
                {t("schedulesPage.actor")}
              </th>
              <th>{t("schedulesPage.state")}</th>
              <th className="table-mobile-hidden">
                {t("schedulesPage.finished")}
              </th>
              <th className="table-actions">
                <span className="visually-hidden">{t("common.actions")}</span>
              </th>
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
                        run.schedule_id
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
                <td className="table-mobile-hidden">
                  <ResourceLink href={links.user(run.user_id)}>
                    {run.username}
                  </ResourceLink>
                </td>
                <td>
                  <ExecutionState state={run.state} />
                  <span className="row-description">{run.message}</span>
                </td>
                <td className="table-mobile-hidden">{when(run.finished_at)}</td>
                <td className="table-actions">
                  {[ExecutionStatus.Failed, ExecutionStatus.Interrupted].some(
                    (value) => value === run.state,
                  ) &&
                    [
                      ServerAction.Install,
                      ServerAction.Start,
                      ServerAction.Stop,
                      ServerAction.Restart,
                      ServerAction.Kill,
                      ServerAction.Backup,
                    ].some((value) => value === run.action) &&
                    servers.items.some(
                      (server) =>
                        server.id === run.server_id &&
                        canRetryExecution(
                          server,
                          run.action,
                          !!run.schedule_id,
                        ),
                    ) && (
                      <Button
                        iconOnly
                        aria-label={retryLabel}
                        title={retryLabel}
                        disabled={busy}
                        onClick={() => {
                          void retry(run);
                        }}
                      >
                        <NavigationIcon name="restart" />
                      </Button>
                    )}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        <Pagination
          pagination={history.pagination}
          onPageChange={navigation.setPage}
          loading={loading || busy}
        />
      </ContentPanel>
      {details && (
        <ScheduleDetailsDrawer
          key={details.id}
          schedule={details}
          servers={servers.items}
          onClose={() => setDetails(null)}
          onSaved={() => {
            void schedules.refresh();
          }}
        />
      )}
      {open && (
        <ScheduleDrawer
          schedule={null}
          servers={managed}
          onClose={() => {
            setOpen(false);
          }}
          onSaved={() => {
            void schedules.refresh();
          }}
        />
      )}
    </section>
  );
}
