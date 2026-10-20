import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
import { useState } from "react";
import { bytes, containerFresh, diskFresh } from "../api/monitoring";
import { useMonitoring } from "../hooks/useMonitoring";
import { useAuthentication } from "../hooks/useAuthentication";
import {
  ResourceReading,
  ResourceReadings,
} from "../components/servers/ResourceReadings";
import { ServerState } from "../components/servers/ServerState";
import { MetricTile, ResourceTile } from "../components/metrics/MetricTile";

import { Table, TableRow } from "../components/table/Table";
import { SparkConnectionState } from "../components/sparks/SparkConnectionState";
import { ServerDetailsDialog } from "../dialogs/ServerDetailsDialog";
import { SparkDetailsDialog } from "../dialogs/SparkDetailsDialog";
import "./monitoring.css";

export function DashboardPage() {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const [details, setDetails] = useState<{
    kind: "server" | "spark";
    id: string;
    name: string;
  } | null>(null);
  const monitor = useMonitoring();
  const auth = useAuthentication();
  const servers = monitor.snapshot?.servers ?? [];
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
  const allocatedCPU = servers.reduce(
    (sum, server) => sum + server.cpu_limit,
    0,
  );
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
        actions={
          <>
            <Button onClick={monitor.refresh}>{t("common.refresh")}</Button>
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
      <div className="monitoring-cards">
        <MetricTile
          label={t("common.servers")}
          value={monitor.loading ? "…" : servers.length}
          detail={
            <>
              <Trans
                i18nKey="dashboardPage.runningUnknown"
                t={t}
                components={{
                  running: <>{running}</>,
                  serversFilterServer: (
                    <>
                      {
                        servers.filter(
                          (server) => server.usage.state === "unknown",
                        ).length
                      }
                    </>
                  ),
                }}
              />
            </>
          }
        />
        <ResourceTile
          label={t("dashboardPage.cpuInUse")}
          used={cpuKnown.length ? cpu : null}
          allocated={allocatedCPU}
          format={formatCPU}
          unit="cores"
        />
        <ResourceTile
          label={t("dashboardPage.memoryInUse")}
          used={memoryKnown.length ? memory : null}
          allocated={allocatedMemory}
          format={bytes}
        />
        <ResourceTile
          label={t("dashboardPage.gameDataSize")}
          used={diskKnown.length ? disk : null}
          allocated={allocatedDisk}
          format={bytes}
        />
      </div>
      <ContentPanel>
        <Table
          className="dashboard-server-table"
          empty={servers.length === 0}
          loading={monitor.loading}
          error={monitor.error}
          resource={t("dashboardPage.servers")}
          emptyDescription={t(
            "dashboardPage.createOrAskToBeAssignedAServerToSee",
          )}
          onRetry={monitor.refresh}
        >
          <caption className="visually-hidden">
            {t("dashboardPage.measuredServerResources")}
          </caption>
          <thead>
            <tr>
              <th>{t("common.server")}</th>
              <th>{t("dashboardPage.state")}</th>
              <th>{t("dashboardPage.measuredUsageAllocation")}</th>
            </tr>
          </thead>
          <tbody>
            {servers.map((server) => (
              <TableRow
                key={server.id}
                onOpen={() => {
                  window.location.hash = `servers/${encodeURIComponent(server.id)}/console`;
                }}
              >
                <td>
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
                  <ServerState state={server.usage.state} />
                  <span className="row-description">
                    {server.usage.container_error}
                  </span>
                </td>
                <td>
                  <ResourceReadings
                    usage={server.usage}
                    cpuLimit={server.cpu_limit}
                    memoryMB={server.memory_mb}
                    diskMB={server.disk_mb}
                    elapsed={monitor.elapsed}
                    layout="grid"
                  />
                </td>
              </TableRow>
            ))}
          </tbody>
        </Table>
      </ContentPanel>
      {auth.session?.admin && (
        <>
          <h2 className="monitoring-heading">{t("common.sparks")}</h2>
          <ContentPanel>
            <Table
              className="dashboard-server-table"
              empty={!monitor.snapshot?.sparks?.length}
              loading={monitor.loading}
              error={monitor.error}
              resource={t("common.sparks")}
              onRetry={monitor.refresh}
            >
              <caption className="visually-hidden">
                {t("dashboardPage.sparkConnectionsAndConfiguredCapacity")}
              </caption>
              <thead>
                <tr>
                  <th>{t("common.spark")}</th>
                  <th>{t("dashboardPage.connection")}</th>
                  <th>{t("dashboardPage.serverAllocationsCapacity")}</th>
                </tr>
              </thead>
              <tbody>
                {(monitor.snapshot?.sparks ?? []).map((spark) => {
                  const hosted = servers.filter(
                    (server) => server.spark_id === spark.id,
                  );
                  return (
                    <TableRow
                      key={spark.id}
                      onOpen={() =>
                        setDetails({
                          kind: "spark",
                          id: spark.id,
                          name: spark.name,
                        })
                      }
                    >
                      <td>
                        <button
                          type="button"
                          className="table-row-trigger"
                          onClick={() =>
                            setDetails({
                              kind: "spark",
                              id: spark.id,
                              name: spark.name,
                            })
                          }
                          aria-label={i18n.t("dashboardPage.viewDetailsFor", {
                            sparkName: spark.name,
                          })}
                        >
                          {spark.name}
                        </button>
                        <span className="row-description">
                          <Trans
                            i18nKey="dashboardPage.servers2"
                            t={i18n.t}
                            components={{ hostedLength: <>{hosted.length}</> }}
                          />
                        </span>
                      </td>
                      <td>
                        <SparkConnectionState
                          disabled={spark.disabled}
                          connected={spark.connected}
                          version={spark.agent_version}
                        />
                      </td>
                      <td>
                        <div className="resource-readings resource-readings-grid">
                          <ResourceReading
                            label={i18n.t("common.cpu")}
                            format={(value) => `${formatCPU(value)} cores`}
                            value={hosted.reduce(
                              (sum, server) => sum + server.cpu_limit,
                              0,
                            )}
                            allocation={spark.cpu_limit}
                            fresh
                            compact
                            detail={i18n.t(
                              "dashboardPage.cpuReservedByServersAgainstThisSparkSConfiguredCapacity",
                            )}
                          />
                          <ResourceReading
                            label={i18n.t("common.memory")}
                            format={bytes}
                            value={hosted.reduce(
                              (sum, server) => sum + server.memory_mb * 1048576,
                              0,
                            )}
                            allocation={spark.memory_mb * 1048576}
                            fresh
                            compact
                            detail={i18n.t(
                              "dashboardPage.memoryReservedByServersAgainstThisSparkSConfiguredCapacity",
                            )}
                          />
                          <ResourceReading
                            label={i18n.t("dashboardPage.gameData")}
                            format={bytes}
                            value={hosted.reduce(
                              (sum, server) => sum + server.disk_mb * 1048576,
                              0,
                            )}
                            allocation={spark.disk_mb * 1048576}
                            fresh
                            compact
                            detail={i18n.t(
                              "dashboardPage.gameDataStorageReservedByServersAgainstThisSparkS",
                            )}
                          />
                        </div>
                      </td>
                    </TableRow>
                  );
                })}
              </tbody>
            </Table>
          </ContentPanel>
        </>
      )}
      <p className="list-summary">
        {t(
          "dashboardPage.usageRefreshesAboutEverySecondCPUAndMemoryReadingsOlder",
        )}
      </p>
      {details?.kind === "spark" && (
        <SparkDetailsDialog
          key={details.id}
          id={details.id}
          title={details.name}
          onClose={() => setDetails(null)}
        />
      )}
      {details?.kind === "server" && (
        <ServerDetailsDialog
          key={details.id}
          id={details.id}
          title={
            <>
              <span>{details.name}</span>
              <ServerState
                state={
                  servers.find((server) => server.id === details.id)?.usage
                    .state ?? "unknown"
                }
              />
            </>
          }
          onClose={() => setDetails(null)}
        />
      )}
    </section>
  );
}
