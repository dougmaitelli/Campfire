import type { Spark } from "../../api/sparks";
import { Table, TableRow } from "../table/Table";
import { SparkConnectionState } from "./SparkConnectionState";

const dateFormat = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
});

const featureLabels: Record<string, string> = {
  "server.configure.v1": "Server configuration",
  "server.ember.behavior.v1": "Extended Ember behavior",
  "server.lifecycle.v1": "Server controls",
  "server.files.v1": "File management",
  "server.backups.v1": "Local backups",
  "server.backups.delete.v1": "Backup deletion",
  "server.backups.remote.v1": "S3 backup restoration",
  "server.metrics.v1": "Usage monitoring",
  "server.console.stream.v1": "Live console",
  "server.disk-quota.v1": "Disk quotas",
  "server.transfer.v1": "Server transfers",
  "server.upgrade.v1": "Updates and reinstalls",
};

export function SparksTable({
  sparks,
  onDetails,
  onEnroll,
  onProvisioning,
}: {
  sparks: Spark[];
  onDetails: (spark: Spark) => void;
  onEnroll: (spark: Spark) => void;
  onProvisioning: (spark: Spark) => void;
}) {
  return (
    <Table className="sparks-table">
      <caption className="visually-hidden">Registered Sparks</caption>
      <thead>
        <tr>
          <th scope="col">Name</th>
          <th scope="col">Status</th>
          <th scope="col">Limits</th>
          <th scope="col">Game networking</th>
          <th scope="col">Added</th>
          <th scope="col" className="table-actions">
            <span className="visually-hidden">Actions</span>
          </th>
        </tr>
      </thead>
      <tbody>
        {sparks.map((spark) => (
          <TableRow key={spark.id} onOpen={() => onDetails(spark)}>
            <td>
              <button
                type="button"
                className="table-row-trigger spark-name"
                onClick={() => onDetails(spark)}
                aria-label={`View details for ${spark.name}`}
              >
                {spark.name}
              </button>
              {spark.description && (
                <span className="spark-description">{spark.description}</span>
              )}
              <span className="spark-id">{spark.id}</span>
            </td>
            <td>
              <SparkConnectionState
                disabled={spark.disabled}
                connected={spark.connected}
              />
              {spark.connected && (
                <details>
                  <summary>
                    Features ({spark.capabilities?.length ?? 0})
                  </summary>
                  <p>
                    Spark {spark.agent_version ?? "unknown version"} · Wire
                    protocol {spark.protocol ?? "unknown"}
                  </p>
                  {spark.capabilities?.length ? (
                    <ul>
                      {spark.capabilities.map((feature) => (
                        <li key={feature} title={feature}>
                          {featureLabels[feature] ?? feature}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p>
                      No features negotiated. Update this Spark to enable
                      operations.
                    </p>
                  )}
                </details>
              )}
            </td>
            <td>
              <span className="resource-summary">
                CPU:{" "}
                {spark.cpu_limit === 0
                  ? "No limit"
                  : `${spark.cpu_limit} cores`}
              </span>
              <span className="resource-summary">
                Memory: {formatMemory(spark.memory_mb)}
              </span>
              <span className="resource-summary">
                Disk: {formatMemory(spark.disk_mb)}
              </span>
            </td>
            <td>
              <span className="resource-summary">
                {spark.game_address || "No game address"}
              </span>
              <span className="resource-summary">
                {spark.port_allocations.length} port{" "}
                {spark.port_allocations.length === 1 ? "range" : "ranges"}
              </span>
            </td>
            <td>
              <time dateTime={spark.created_at}>
                {dateFormat.format(new Date(spark.created_at))}
              </time>
            </td>
            <td className="table-actions">
              <div className="row-actions">
                <button
                  className="button"
                  onClick={() => onProvisioning(spark)}
                  aria-label={`Provisioning access for ${spark.name}`}
                >
                  Provisioning
                </button>
                <button
                  type="button"
                  className="button"
                  disabled={spark.disabled}
                  onClick={() => onEnroll(spark)}
                  aria-label={`Enroll ${spark.name}`}
                >
                  Enroll
                </button>
              </div>
            </td>
          </TableRow>
        ))}
      </tbody>
    </Table>
  );
}

function formatMemory(mib: number) {
  if (mib === 0) return "No limit";
  return mib >= 1024
    ? `${(mib / 1024).toLocaleString(undefined, { maximumFractionDigits: 2 })} GiB`
    : `${mib} MiB`;
}
