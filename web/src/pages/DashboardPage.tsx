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
        title="Dashboard"
        description={
          <>
            Live usage for{" "}
            {auth.session?.admin ? "all servers" : "your accessible servers"}.
          </>
        }
        actions={
          <>
            <Button onClick={monitor.refresh}>Refresh</Button>
          </>
        }
      />
      {monitor.error &&
        (servers.length > 0 || !!monitor.snapshot?.sparks?.length) && (
          <InlineMessage role="alert">
            {monitor.error} Previous readings may be stale.
          </InlineMessage>
        )}
      {monitor.snapshot?.incomplete && (
        <InlineMessage tone="info" role="status">
          Some servers are not reporting resource usage.
        </InlineMessage>
      )}
      <div className="monitoring-cards">
        <MetricTile
          label="Servers"
          value={monitor.loading ? "…" : servers.length}
          detail={
            <>
              {running} running ·{" "}
              {
                servers.filter((server) => server.usage.state === "unknown")
                  .length
              }{" "}
              unknown
            </>
          }
        />
        <ResourceTile
          label="CPU in use"
          used={cpuKnown.length ? cpu : null}
          allocated={allocatedCPU}
          format={formatCPU}
          unit="cores"
        />
        <ResourceTile
          label="Memory in use"
          used={memoryKnown.length ? memory : null}
          allocated={allocatedMemory}
          format={bytes}
        />
        <ResourceTile
          label="Game-data size"
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
          resource="servers"
          emptyDescription="Create or ask to be assigned a server to see its usage."
          onRetry={monitor.refresh}
        >
          <caption className="visually-hidden">
            Measured server resources
          </caption>
          <thead>
            <tr>
              <th>Server</th>
              <th>State</th>
              <th>Measured usage / allocation</th>
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
                    aria-label={`Open ${server.name}`}
                  >
                    {server.name}
                  </button>
                  <span className="row-description">{server.spark_name}</span>
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
          <h2 className="monitoring-heading">Sparks</h2>
          <ContentPanel>
            <Table
              className="dashboard-server-table"
              empty={!monitor.snapshot?.sparks?.length}
              loading={monitor.loading}
              error={monitor.error}
              resource="Sparks"
              onRetry={monitor.refresh}
            >
              <caption className="visually-hidden">
                Spark connections and configured capacity
              </caption>
              <thead>
                <tr>
                  <th>Spark</th>
                  <th>Connection</th>
                  <th>Server allocations / capacity</th>
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
                          aria-label={`View details for ${spark.name}`}
                        >
                          {spark.name}
                        </button>
                        <span className="row-description">
                          {hosted.length} servers
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
                            label="CPU"
                            format={(value) => `${formatCPU(value)} cores`}
                            value={hosted.reduce(
                              (sum, server) => sum + server.cpu_limit,
                              0,
                            )}
                            allocation={spark.cpu_limit}
                            fresh
                            compact
                            detail="CPU reserved by servers against this Spark's configured capacity."
                          />
                          <ResourceReading
                            label="Memory"
                            format={bytes}
                            value={hosted.reduce(
                              (sum, server) => sum + server.memory_mb * 1048576,
                              0,
                            )}
                            allocation={spark.memory_mb * 1048576}
                            fresh
                            compact
                            detail="Memory reserved by servers against this Spark's configured capacity."
                          />
                          <ResourceReading
                            label="Game data"
                            format={bytes}
                            value={hosted.reduce(
                              (sum, server) => sum + server.disk_mb * 1048576,
                              0,
                            )}
                            allocation={spark.disk_mb * 1048576}
                            fresh
                            compact
                            detail="Game-data storage reserved by servers against this Spark's configured capacity."
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
        Usage refreshes about every second. CPU and memory readings older than
        15 seconds are stale; game-data scans refresh every 30 seconds.
        Game-data size excludes backups and container images. Filesystem quotas
        limit allocated game-data blocks; reported logical file sizes can
        differ.
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
