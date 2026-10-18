import { Button } from "../forms/Button";
import type { Spark } from "../../api/sparks";
import { Table, TableRow } from "../table/Table";
import { SparkConnectionState } from "./SparkConnectionState";
import {
  EMPTY_VALUE,
  formatDisplayValue,
} from "../../utils/formatDisplayValue";
import { PortPill } from "../network/PortPill";
import { SparkResourceReadings } from "./SparkResourceReadings";
import type { MonitoredServer } from "../../api/monitoring";

export function SparksTable({
  sparks,
  onDetails,
  loading,
  error,
  onRetry,
  onAdd,
  onUpdate,
  updating,
  servers,
  elapsed,
}: {
  sparks: Spark[];
  onDetails: (spark: Spark) => void;
  loading: boolean;
  error: string | null;
  onRetry: () => void;
  onAdd: () => void;
  onUpdate: (spark: Spark) => void;
  updating: string | null;
  servers: MonitoredServer[] | undefined;
  elapsed: number;
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
          <th scope="col">Resources</th>
          <th scope="col">Network</th>
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
                version={spark.agent_version}
              />
            </td>
            <td>
              <SparkResourceReadings
                spark={spark}
                servers={servers}
                elapsed={elapsed}
              />
            </td>
            <td>
              <span className="resource-summary">
                {formatDisplayValue(spark.game_address)}
              </span>
              <div className="pill-group">
                {spark.port_allocations.map((allocation, index) => (
                  <PortPill
                    key={index}
                    address={allocation.bind_ip}
                    port={
                      allocation.start === allocation.end
                        ? allocation.start
                        : `${allocation.start}–${allocation.end}`
                    }
                    protocol={allocation.protocol}
                  />
                ))}
                {spark.port_allocations.length === 0 && EMPTY_VALUE}
              </div>
            </td>
            <td className="table-actions">
              {spark.agent_update_available && (
                <Button
                  variant="warning"
                  disabled={
                    updating !== null ||
                    !spark.capabilities?.includes("spark.self-update.v1")
                  }
                  onClick={() => onUpdate(spark)}
                  title={
                    spark.capabilities?.includes("spark.self-update.v1")
                      ? `Update to ${spark.latest_agent_version ?? "the latest release"}`
                      : "This client needs a manual upgrade before it supports remote updates."
                  }
                  loading={updating === spark.id}
                  loadingLabel="Updating…"
                >
                  Update Spark
                </Button>
              )}
            </td>
          </TableRow>
        ))}
      </tbody>
    </Table>
  );
}
