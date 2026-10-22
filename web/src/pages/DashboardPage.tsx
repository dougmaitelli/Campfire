import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { ContentState } from "../components/layout/ContentState";
import { OverviewEmptyState } from "../components/overview/OverviewEmptyState";
import { SectionHeading } from "../components/layout/SectionHeading";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { ArrowLink } from "../components/navigation/ArrowLink";
import { SearchField } from "../components/forms/SearchField";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
import { useCallback, useState } from "react";
import { bytes, containerFresh, diskFresh } from "../api/monitoring";
import { useMonitoring } from "../hooks/useMonitoring";
import { useAuthentication } from "../hooks/useAuthentication";
import { ResourceReadings } from "../components/servers/ResourceReadings";
import { ServerState } from "../components/servers/ServerState";
import { GameArt } from "../components/media/GameArt";
import { ServerQuickControls } from "../components/servers/ServerQuickControls";
import { useServerRuntime } from "../hooks/useServerRuntime";
import { MetricTile } from "../components/metrics/MetricTile";
import { MetricTiles } from "../components/metrics/MetricTiles";
import { ResourceReading } from "../components/metrics/ResourceReading";

import { Table, TableRow } from "../components/table/Table";
import { listAllBackups } from "../api/backups";
import { listSchedules } from "../api/activity";
import { listServers } from "../api/servers";
import { useCollection } from "../hooks/useCollection";
import { BackupState } from "../components/servers/BackupState";
import { ScheduleToggle } from "../components/schedules/ScheduleToggle";
import { ScheduleNextRun } from "../components/schedules/ScheduleNextRun";
import { EMPTY_VALUE } from "../utils/formatDisplayValue";
import { cpuUsageDivisor, formatCPUUsage } from "../utils/cpuUsage";
import "./dashboard.css";
import "./monitoring.css";

const OVERVIEW_REFRESH_INTERVAL_MS = 10000;

export function DashboardPage() {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const monitor = useMonitoring();
  const auth = useAuthentication();
  const canReadBackups =
    auth.session?.permissions?.includes("servers.manage_all") === true;
  const loadBackups = useCallback(
    (signal: AbortSignal) =>
      canReadBackups ? listAllBackups(signal) : Promise.resolve([]),
    [canReadBackups],
  );
  const backups = useCollection(loadBackups, OVERVIEW_REFRESH_INTERVAL_MS);
  const schedules = useCollection(listSchedules, OVERVIEW_REFRESH_INTERVAL_MS);
  const serverInventory = useCollection(
    listServers,
    OVERVIEW_REFRESH_INTERVAL_MS,
  );
  const runtime = useServerRuntime(
    serverInventory.items
      .filter((server) => server.permissions.includes("server.operate"))
      .map((server) => server.id),
  );
  const [serverActionError, setServerActionError] = useState<string | null>(
    null,
  );
  const [scheduleError, setScheduleError] = useState<string | null>(null);
  const [serverSearch, setServerSearch] = useState("");
  const failedBackups = backups.items.filter((backup) =>
    ["failed", "error", "unavailable"].includes(backup.state),
  ).length;
  const recentBackups = [...backups.items]
    .sort((a, b) => Date.parse(b.modified_at) - Date.parse(a.modified_at))
    .slice(0, 5);
  const upcomingSchedules = schedules.items
    .filter((schedule) => schedule.enabled && schedule.next_run_at > 0)
    .sort((a, b) => a.next_run_at - b.next_run_at)
    .slice(0, 5);
  const servers = monitor.snapshot?.servers ?? [];
  const serverArtURLs = new Map(
    serverInventory.items.map((server) => [server.id, server.ember_art_url]),
  );
  const hasGameArt = servers.some((server) => serverArtURLs.get(server.id));
  const hasServers = servers.length > 0 || serverInventory.items.length > 0;
  const inventoryReady = !serverInventory.loading && !serverInventory.error;
  const visibleServers = servers.filter((server) =>
    server.name.toLowerCase().includes(serverSearch.toLowerCase()),
  );
  const cpuKnown = servers.filter(
    (server) =>
      server.usage.cpu_percent !== null &&
      containerFresh(server.usage, monitor.elapsed),
  );
  const memoryKnown = servers.filter(
    (server) =>
      server.usage.memory_bytes !== null &&
      containerFresh(server.usage, monitor.elapsed),
  );
  const diskKnown = servers.filter(
    (server) =>
      server.usage.disk_bytes !== null &&
      diskFresh(server.usage, monitor.elapsed),
  );
  const running = servers.filter(
    (server) =>
      server.usage.state === "running" &&
      containerFresh(server.usage, monitor.elapsed),
  ).length;
  const cpu = cpuKnown.reduce(
    (sum, server) => sum + (server.usage.cpu_percent ?? 0) / 100,
    0,
  );
  const memory = memoryKnown.reduce(
    (sum, server) => sum + (server.usage.memory_bytes ?? 0),
    0,
  );
  const disk = diskKnown.reduce(
    (sum, server) => sum + (server.usage.disk_bytes ?? 0),
    0,
  );
  const allocatedCPU = servers.every((server) => server.cpu_limit > 0)
    ? servers.reduce((sum, server) => sum + server.cpu_limit, 0)
    : 0;
  const allocatedMemory = servers.reduce(
    (sum, server) => sum + server.memory_mb * 1048576,
    0,
  );
  const allocatedDisk = servers.reduce(
    (sum, server) => sum + server.disk_mb * 1048576,
    0,
  );
  const formatCPU = (value: number) =>
    value.toLocaleString(undefined, { maximumFractionDigits: 2 });
  const enabledSchedules = schedules.items.filter(
    (schedule) => schedule.enabled,
  );
  const nextSchedule = enabledSchedules
    .filter((schedule) => schedule.next_run_at > 0)
    .sort((a, b) => a.next_run_at - b.next_run_at)[0];
  if (!hasServers && inventoryReady) {
    return (
      <OverviewEmptyState
        sparkCount={
          monitor.snapshot ? (monitor.snapshot.sparks?.length ?? 0) : undefined
        }
        canManageSparks={
          auth.session?.permissions?.includes("sparks.manage") === true
        }
        canManageEmbers={
          auth.session?.permissions?.includes("embers.manage") === true
        }
        canCreateServers={
          auth.session?.admin === true ||
          auth.session?.permissions?.includes("servers.create") === true
        }
      />
    );
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.dashboard")}
        description={
          <>
            <Trans
              i18nKey="dashboardPage.liveUsageFor"
              t={t}
              components={{
                authSessionAdmin: (
                  <>
                    {auth.session?.admin
                      ? t("dashboardPage.allServers")
                      : t("dashboardPage.yourAccessibleServers")}
                  </>
                ),
              }}
            />
          </>
        }
      />
      {monitor.error &&
        (servers.length > 0 || !!monitor.snapshot?.sparks?.length) && (
          <InlineMessage role="alert">
            <Trans
              i18nKey="dashboardPage.previousReadingsMayBeStale"
              t={t}
              components={{ monitorError: <>{monitor.error}</> }}
            />
          </InlineMessage>
        )}
      {monitor.snapshot?.incomplete && (
        <InlineMessage tone="info" role="status">
          {t("dashboardPage.someServersAreNotReportingResourceUsage")}
        </InlineMessage>
      )}
      {hasServers && (
        <MetricTiles>
          <MetricTile
            label={t("common.servers")}
            icon={<NavigationIcon name="servers" />}
            value={monitor.loading ? "…" : servers.length}
            detail={t("dashboardPage.runningCount", { count: running })}
          />
          <ResourceReading
            label={t("common.cpu")}
            icon={<NavigationIcon name="cpu" />}
            value={cpuKnown.length ? cpu : null}
            allocation={allocatedCPU}
            format={formatCPU}
            formatUsed={(value) =>
              formatCPUUsage(
                (value * 100) / cpuUsageDivisor(allocatedCPU),
                allocatedCPU,
              )
            }
            unit="cores"
          />
          <ResourceReading
            label={t("common.memory")}
            icon={<NavigationIcon name="memory" />}
            value={memoryKnown.length ? memory : null}
            allocation={allocatedMemory}
            format={bytes}
          />
          <ResourceReading
            label={t("common.disk")}
            icon={<NavigationIcon name="disk" />}
            value={diskKnown.length ? disk : null}
            allocation={allocatedDisk}
            format={bytes}
          />
          <MetricTile
            label={t("common.backups")}
            icon={<NavigationIcon name="backups" />}
            value={
              !canReadBackups || backups.error
                ? EMPTY_VALUE
                : backups.loading
                  ? "…"
                  : backups.items.length === 0
                    ? 0
                    : `${backups.items.filter((backup) => backup.state === "available").length} / ${backups.items.length}`
            }
            detail={
              !canReadBackups ? (
                t("dashboardPage.restricted")
              ) : backups.error ? (
                t("dashboardPage.summaryUnavailable")
              ) : backups.loading ? null : backups.items.length === 0 ? (
                t("backupsPage.noBackupsYet")
              ) : (
                <span
                  className={`backup-summary-status${failedBackups > 0 ? " backup-summary-status-failed" : ""}`}
                >
                  <NavigationIcon
                    name={failedBackups > 0 ? "x-circle" : "check-circle"}
                  />
                  {failedBackups > 0
                    ? t("dashboardPage.failedBackups", { count: failedBackups })
                    : t("dashboardPage.successful")}
                </span>
              )
            }
          />
          <MetricTile
            label={t("common.schedules")}
            icon={<NavigationIcon name="schedules" />}
            value={
              schedules.error
                ? EMPTY_VALUE
                : schedules.loading
                  ? "…"
                  : t("dashboardPage.enabledCount", {
                      count: enabledSchedules.length,
                    })
            }
            detail={
              schedules.error ? (
                t("dashboardPage.summaryUnavailable")
              ) : (
                <>
                  {!schedules.loading &&
                    enabledSchedules.length === 0 &&
                    t("schedulesPage.noSchedulesYet")}
                  {nextSchedule && (
                    <>
                      {t("dashboardPage.nextScheduledRun", {
                        time: new Date(
                          nextSchedule.next_run_at * 1000,
                        ).toLocaleString(undefined, {
                          month: "short",
                          day: "numeric",
                          hour: "numeric",
                          minute: "2-digit",
                        }),
                      })}
                    </>
                  )}
                </>
              )
            }
          />
        </MetricTiles>
      )}
      {!hasServers ? (
        <ContentState
          loading={serverInventory.loading}
          error={serverInventory.error}
          resource={t("common.servers")}
          onRetry={() => void serverInventory.refresh()}
        />
      ) : (
        <>
          <ContentPanel>
            <SectionHeading
              title={t("common.servers")}
              actions={
                <SearchField
                  label={
                    <span className="visually-hidden">
                      {t("serversPage.searchServers")}
                    </span>
                  }
                  placeholder={t("serversPage.searchServers")}
                  value={serverSearch}
                  onChange={(event) => setServerSearch(event.target.value)}
                />
              }
            />
            {serverActionError && (
              <InlineMessage role="alert">{serverActionError}</InlineMessage>
            )}
            <Table
              className="dashboard-server-table"
              empty={visibleServers.length === 0}
              {...(serverSearch
                ? { emptyMessage: t("serversPage.noMatchingServers") }
                : {
                    emptyDescription: t(
                      "dashboardPage.createOrAskToBeAssignedAServerToSee",
                    ),
                  })}
              loading={monitor.loading}
              error={monitor.error}
              resource={t("dashboardPage.servers")}
              onRetry={monitor.refresh}
            >
              <caption className="visually-hidden">
                {t("dashboardPage.measuredServerResources")}
              </caption>
              <thead>
                <tr>
                  {hasGameArt && <th className="game-art-cell" />}
                  <th className={hasGameArt ? "game-art-name" : undefined}>
                    {t("common.server")}
                  </th>
                  <th>{t("dashboardPage.state")}</th>
                  <th>{t("common.cpu")}</th>
                  <th>{t("common.memory")}</th>
                  <th>{t("common.disk")}</th>
                  <th className="table-actions">
                    <span className="visually-hidden">
                      {t("common.actions")}
                    </span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {visibleServers.map((server) => (
                  <TableRow
                    key={server.id}
                    onOpen={() => {
                      window.location.hash = `servers/${encodeURIComponent(server.id)}/console`;
                    }}
                  >
                    {hasGameArt && (
                      <td className="game-art-cell">
                        <GameArt src={serverArtURLs.get(server.id)} />
                      </td>
                    )}
                    <td className={hasGameArt ? "game-art-name" : undefined}>
                      <button
                        type="button"
                        className="table-row-trigger"
                        onClick={() => {
                          window.location.hash = `servers/${encodeURIComponent(server.id)}/console`;
                        }}
                        aria-label={i18n.t("dashboardPage.open", {
                          serverName: server.name,
                        })}
                      >
                        {server.name}
                      </button>
                      <span className="row-description">
                        <ResourceLink href={links.spark(server.spark_id)}>
                          {server.spark_name}
                        </ResourceLink>
                      </span>
                    </td>
                    <td>
                      <ServerState
                        now={
                          (monitor.snapshot?.collected_at ?? 0) +
                          monitor.elapsed
                        }
                        state={
                          runtime.statuses[server.id]?.state ??
                          server.usage.state
                        }
                        startedAt={
                          server.usage.state === "running"
                            ? server.usage.started_at
                            : undefined
                        }
                        stoppedAt={
                          ["stopped", "exited"].includes(server.usage.state)
                            ? server.usage.stopped_at
                            : undefined
                        }
                      />
                      <span className="row-description">
                        {server.usage.container_error}
                      </span>
                    </td>
                    <ResourceReadings
                      usage={server.usage}
                      cpuLimit={server.cpu_limit}
                      memoryMB={server.memory_mb}
                      diskMB={server.disk_mb}
                      elapsed={monitor.elapsed}
                      layout="columns"
                    />
                    <td className="table-actions">
                      <ServerQuickControls
                        id={server.id}
                        name={server.name}
                        status={runtime.statuses[server.id]}
                        canOperate={serverInventory.items.some(
                          (item) =>
                            item.id === server.id &&
                            item.permissions.includes("server.operate"),
                        )}
                        disabled={
                          monitor.snapshot?.sparks?.some(
                            (spark) =>
                              spark.id === server.spark_id && spark.disabled,
                          ) === true
                        }
                        onStatus={runtime.upsert}
                        onError={setServerActionError}
                      />
                    </td>
                  </TableRow>
                ))}
              </tbody>
            </Table>
          </ContentPanel>
          <div className="dashboard-activity-grid">
            <ContentPanel>
              <SectionHeading
                title={t("dashboardPage.recentBackups")}
                icon={<NavigationIcon name="backups" />}
                actions={
                  canReadBackups && (
                    <ArrowLink href="#backups">
                      {t("dashboardPage.viewAllBackups")}
                    </ArrowLink>
                  )
                }
              />
              {!canReadBackups ? (
                <p className="field-help">
                  {t("dashboardPage.backupCatalogRestricted")}
                </p>
              ) : (
                <>
                  {backups.error && backups.items.length > 0 && (
                    <InlineMessage role="alert">{backups.error}</InlineMessage>
                  )}
                  <Table
                    empty={recentBackups.length === 0}
                    loading={backups.loading}
                    error={backups.error}
                    resource={t("common.backups")}
                    emptyMessage={t("backupsPage.noBackupsYet")}
                    onRetry={() => void backups.refresh()}
                  >
                    <caption className="visually-hidden">
                      {t("dashboardPage.recentBackups")}
                    </caption>
                    <thead>
                      <tr>
                        <th>{t("common.server")}</th>
                        <th>{t("backupsPage.created")}</th>
                        <th>{t("backupsPage.size")}</th>
                        <th>{t("common.status")}</th>
                      </tr>
                    </thead>
                    <tbody>
                      {recentBackups.map((backup) => (
                        <tr key={backup.id}>
                          <td>
                            <ResourceLink
                              href={links.server(backup.server_id, "backups")}
                            >
                              {backup.server_name}
                            </ResourceLink>
                          </td>
                          <td>
                            <time dateTime={backup.modified_at}>
                              {new Date(backup.modified_at).toLocaleString()}
                            </time>
                          </td>
                          <td>{bytes(backup.size)}</td>
                          <td>
                            <BackupState state={backup.state} />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </Table>
                </>
              )}
            </ContentPanel>
            <ContentPanel>
              <SectionHeading
                title={t("dashboardPage.upcomingSchedules")}
                icon={<NavigationIcon name="schedules" />}
                actions={
                  <ArrowLink href="#schedules">
                    {t("dashboardPage.viewAllSchedules")}
                  </ArrowLink>
                }
              />
              {scheduleError && (
                <InlineMessage role="alert">{scheduleError}</InlineMessage>
              )}
              {schedules.error && schedules.items.length > 0 && (
                <InlineMessage role="alert">{schedules.error}</InlineMessage>
              )}
              <Table
                empty={upcomingSchedules.length === 0}
                loading={schedules.loading}
                error={schedules.error}
                resource={t("common.schedules")}
                emptyMessage={t("dashboardPage.noUpcomingSchedules")}
                onRetry={() => void schedules.refresh()}
              >
                <caption className="visually-hidden">
                  {t("dashboardPage.upcomingSchedules")}
                </caption>
                <thead>
                  <tr>
                    <th>{t("schedulesPage.schedule")}</th>
                    <th>{t("common.server")}</th>
                    <th>{t("schedulesPage.nextRun")}</th>
                    <th>{t("common.enabled")}</th>
                  </tr>
                </thead>
                <tbody>
                  {upcomingSchedules.map((schedule) => (
                    <tr key={schedule.id}>
                      <td>
                        <ResourceLink href={links.schedule(schedule.id)}>
                          {schedule.name}
                        </ResourceLink>
                      </td>
                      <td>
                        <ResourceLink href={links.server(schedule.server_id)}>
                          {servers.find(
                            (server) => server.id === schedule.server_id,
                          )?.name ?? EMPTY_VALUE}
                        </ResourceLink>
                      </td>
                      <td>
                        <ScheduleNextRun
                          at={schedule.next_run_at}
                          enabled={schedule.enabled}
                        />
                      </td>
                      <td>
                        <ScheduleToggle
                          schedule={schedule}
                          disabled={
                            !auth.session?.admin &&
                            !serverInventory.items.some(
                              (server) =>
                                server.id === schedule.server_id &&
                                server.permissions.includes(
                                  "server.configure",
                                ) &&
                                server.permissions.includes("server.operate"),
                            )
                          }
                          onSaved={schedules.upsert}
                          onError={setScheduleError}
                        />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </Table>
            </ContentPanel>
          </div>
        </>
      )}
    </section>
  );
}
