import type { Spark } from "../../api/sparks";

const dateFormat = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
});

export function SparksTable({
  sparks,
  onEdit,
  onEnroll,
  onProvisioning,
  onDelete,
  deleting,
}: {
  sparks: Spark[];
  onEdit: (spark: Spark) => void;
  onEnroll: (spark: Spark) => void;
  onProvisioning: (spark: Spark) => void;
  onDelete: (spark: Spark) => void;
  deleting: string | null;
}) {
  return (
    <div className="table-container">
      <table className="sparks-table">
        <caption className="visually-hidden">Registered Sparks</caption>
        <thead>
          <tr>
            <th scope="col">Name</th>
            <th scope="col">Status</th>
            <th scope="col">Limits</th>
            <th scope="col">Game networking</th>
            <th scope="col">Added</th>
            <th scope="col">
              <span className="visually-hidden">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {sparks.map((spark) => (
            <tr key={spark.id}>
              <td>
                <span className="spark-name">{spark.name}</span>
                {spark.description && (
                  <span className="spark-description">{spark.description}</span>
                )}
                <span className="spark-id">{spark.id}</span>
              </td>
              <td>
                <span
                  className={`type-badge spark-status ${spark.disabled ? "is-disabled" : spark.connected ? "is-connected" : ""}`}
                >
                  {spark.disabled
                    ? "Disabled"
                    : spark.connected
                      ? "Connected"
                      : "Disconnected"}
                </span>
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
              <td>
                <div className="row-actions">
                  <button
                    className="button"
                    onClick={() => onProvisioning(spark)}
                    aria-label={`Provisioning access for ${spark.name}`}
                  >
                    Provisioning
                  </button>
                  <button
                    className="button"
                    onClick={() => onEdit(spark)}
                    aria-label={`Edit ${spark.name}`}
                  >
                    Edit
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
                  <button
                    className="button"
                    disabled={deleting !== null}
                    onClick={() => onDelete(spark)}
                    aria-label={`Remove ${spark.name}`}
                  >
                    {deleting === spark.id ? "Removing…" : "Remove"}
                  </button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function formatMemory(mib: number) {
  if (mib === 0) return "No limit";
  return mib >= 1024
    ? `${(mib / 1024).toLocaleString(undefined, { maximumFractionDigits: 2 })} GiB`
    : `${mib} MiB`;
}
