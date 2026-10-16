import { bytes, containerFresh, diskFresh } from "../api/monitoring";
import { useMonitoring } from "../hooks/useMonitoring";
import { useAuthentication } from "../hooks/useAuthentication";
import { ResourceReadings } from "../components/servers/ResourceReadings";
import { ServerState } from "../components/servers/ServerState";
import { MetricTile, ResourceTile } from "../components/metrics/MetricTile";
import "./management.css";
import { Table } from "../components/table/Table";
import "./monitoring.css";

export function DashboardPage() {
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
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Dashboard</h1>
          <p className="page-description">
            Live usage for{" "}
            {auth.session?.admin ? "all servers" : "your accessible servers"}.
          </p>
        </div>
        <div className="page-actions">
          <button className="button" onClick={monitor.refresh}>
            Refresh
          </button>
        </div>
      </header>
      {monitor.error && (
        <p className="error-message" role="alert">
          {monitor.error} Previous readings may be stale.
        </p>
      )}
      {monitor.snapshot?.incomplete && (
        <p className="field-help" role="status">
          Some servers are not reporting resource usage.
        </p>
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
      <div className="management-content">
        <Table className="dashboard-server-table">
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
              <tr key={server.id}>
                <td>
                  <strong>{server.name}</strong>
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
              </tr>
            ))}
            {!servers.length && (
              <tr>
                <td colSpan={3}>
                  {monitor.loading
                    ? "Collecting usage…"
                    : "No accessible servers. Create or ask to be assigned a server to see its usage."}
                </td>
              </tr>
            )}
          </tbody>
        </Table>
      </div>
      {auth.session?.admin && (
        <>
          <h2 className="monitoring-heading">Sparks</h2>
          <div className="management-content">
            <Table>
              <caption className="visually-hidden">
                Spark connections and configured capacity
              </caption>
              <thead>
                <tr>
                  <th>Spark</th>
                  <th>Connection</th>
                  <th>Configured capacity</th>
                  <th>Server allocations</th>
                </tr>
              </thead>
              <tbody>
                {(monitor.snapshot?.sparks ?? []).map((spark) => {
                  const hosted = servers.filter(
                    (server) => server.spark_id === spark.id,
                  );
                  return (
                    <tr key={spark.id}>
                      <td>
                        <strong>{spark.name}</strong>
                        <span className="row-description">
                          {hosted.length} servers
                        </span>
                      </td>
                      <td>
                        {spark.disabled
                          ? "Disabled"
                          : spark.connected
                            ? "Connected"
                            : "Disconnected"}
                      </td>
                      <td>
                        {spark.cpu_limit
                          ? `${spark.cpu_limit} cores`
                          : "No CPU cap"}
                        <span className="resource-line">
                          {spark.memory_mb
                            ? `${bytes(spark.memory_mb * 1048576)} memory`
                            : "No memory cap"}
                        </span>
                        <span className="resource-line">
                          {spark.disk_mb
                            ? `${bytes(spark.disk_mb * 1048576)} game data`
                            : "No disk cap"}
                        </span>
                      </td>
                      <td>
                        {hosted.reduce(
                          (sum, server) => sum + server.cpu_limit,
                          0,
                        )}{" "}
                        cores
                        <span className="resource-line">
                          {bytes(
                            hosted.reduce(
                              (sum, server) => sum + server.memory_mb * 1048576,
                              0,
                            ),
                          )}{" "}
                          memory
                        </span>
                        <span className="resource-line">
                          {bytes(
                            hosted.reduce(
                              (sum, server) => sum + server.disk_mb * 1048576,
                              0,
                            ),
                          )}{" "}
                          game data
                        </span>
                      </td>
                    </tr>
                  );
                })}
                {!monitor.snapshot?.sparks?.length && (
                  <tr>
                    <td colSpan={4}>
                      {monitor.loading
                        ? "Loading Sparks…"
                        : "No Sparks registered."}
                    </td>
                  </tr>
                )}
              </tbody>
            </Table>
          </div>
        </>
      )}
      <p className="list-summary">
        Usage refreshes about every five seconds. CPU and memory readings older
        than 15 seconds are stale; game-data scans refresh every 30 seconds.
        Game-data size excludes backups and container images. Filesystem quotas
        limit allocated game-data blocks; reported logical file sizes can
        differ.
      </p>
    </section>
  );
}
