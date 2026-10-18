import {
  bytes,
  containerFresh,
  diskFresh,
  type ResourceUsage,
} from "../../api/monitoring";
import "./resource-readings.css";
import { ResourceMeter } from "../metrics/ResourceMeter";

export function ResourceReading({
  label,
  value,
  allocation,
  fresh,
  detail,
  compact,
  format,
}: {
  label: string;
  value: number | null;
  allocation: number;
  fresh: boolean;
  detail: string;
  compact: boolean;
  format: (value: number) => string;
}) {
  const measured = value !== null;
  return (
    <div
      className={`usage-reading${measured && !fresh ? " usage-stale" : ""}`}
      title={detail}
    >
      <span className="usage-label">{label}</span>
      <span>
        {measured ? format(value) : "-"}
        <span className="usage-allocation">
          {measured ? " / " : "/"}
          {allocation > 0 ? format(allocation) : "No limit"}
          {compact || allocation <= 0 ? "" : " allocated"}
        </span>
        {measured && !fresh && <span className="usage-age"> · stale</span>}
      </span>
      <ResourceMeter
        label={label}
        used={fresh ? value : null}
        allocated={allocation}
      />
    </div>
  );
}

export function ResourceReadings({
  usage,
  cpuLimit,
  memoryMB,
  diskMB,
  elapsed,
  layout = "stack",
}: {
  usage: ResourceUsage | undefined;
  cpuLimit: number;
  memoryMB: number;
  diskMB: number;
  elapsed: number;
  layout?: "stack" | "grid" | "compact";
}) {
  const cpu = usage?.cpu_percent;
  return (
    <div
      className={`resource-readings${layout !== "stack" ? ` resource-readings-${layout}` : ""}`}
    >
      <ResourceReading
        label="CPU"
        format={(value) =>
          `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })} cores`
        }
        compact={layout !== "stack"}
        value={cpu === undefined || cpu === null ? null : cpu / 100}
        allocation={cpuLimit}
        fresh={containerFresh(usage, elapsed)}
        detail={
          usage !== undefined && usage.container_error.length > 0
            ? usage.container_error
            : cpu === null
              ? "Waiting for a CPU sample."
              : "Measured CPU use; one fully used core is 100%."
        }
      />
      <ResourceReading
        label="Memory"
        format={bytes}
        compact={layout !== "stack"}
        value={usage?.memory_bytes ?? null}
        allocation={memoryMB * 1048576}
        fresh={containerFresh(usage, elapsed)}
        detail={
          usage !== undefined && usage.container_error.length > 0
            ? usage.container_error
            : "Container memory use, excluding reclaimable file cache."
        }
      />
      <ResourceReading
        label="Game data"
        format={bytes}
        compact={layout !== "stack"}
        value={usage?.disk_bytes ?? null}
        allocation={diskMB * 1048576}
        fresh={diskFresh(usage, elapsed)}
        detail={
          usage !== undefined && usage.disk_error.length > 0
            ? usage.disk_error
            : usage?.disk_sampled_at
              ? `Sampled ${new Date(usage.disk_sampled_at * 1000).toLocaleString()}. Backups and images excluded.`
              : "Waiting for a game-data size sample."
        }
      />
    </div>
  );
}
