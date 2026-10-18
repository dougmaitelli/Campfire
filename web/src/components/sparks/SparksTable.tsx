import { Button } from "../forms/Button";
import type { Spark } from "../../api/sparks";
import { Table, TableRow } from "../table/Table";
import { SparkConnectionState } from "./SparkConnectionState";

const dateFormat = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
});

export function SparksTable({
  sparks,
  onDetails,
  loading,
  error,
  onRetry,
  onAdd,
}: {
  sparks: Spark[];
  onDetails: (spark: Spark) => void;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  onAdd: () => void;
}) {
  return (
    <Table
      className="sparks-table"
      empty={sparks.length === 0}
      loading={loading}
      error={error}
      resource="Sparks"
      emptyDescription="Add your first node to get started."
      emptyAction={
        <Button variant="primary" onClick={onAdd}>
          Add Spark
        </Button>
      }
      onRetry={onRetry}
    >
      <caption className="visually-hidden">Registered Sparks</caption>
      <thead>
        <tr>
          <th scope="col">Name</th>
          <th scope="col">Status</th>
          <th scope="col">Limits</th>
          <th scope="col">Game networking</th>
          <th scope="col">Added</th>
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
