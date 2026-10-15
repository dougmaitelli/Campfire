import {
  bytes,
  containerFresh,
  diskFresh,
  type ResourceUsage,
} from "../../api/monitoring";
import "./resource-readings.css";

function Reading({
  label,
  value,
  allocation,
  fresh,
  detail,
}: {
  label: string;
  value: number | null;
  allocation: number;
  fresh: boolean;
  detail: string;
}) {
  const measured = value !== null;
  const format =
    label === "CPU"
      ? (value: number) =>
          `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })} cores`
      : bytes;
  return (
    <div
      className={`usage-reading${measured && !fresh ? " usage-stale" : ""}`}
      title={detail}
    >
      <span className="usage-label">{label}</span>
      <span>
        {measured ? format(value) : "Unavailable"}
        <span className="usage-allocation">
          {" "}
          / {format(allocation)} allocated
        </span>
        {measured && !fresh && <span className="usage-age"> · stale</span>}
      </span>
      {measured && fresh && (
        <meter
          min={0}
          max={allocation || 1}
          value={Math.min(value, allocation || 1)}
          aria-label={`${label} measured usage against allocation`}
        />
      )}
    </div>
  );
}

export function ResourceReadings({
  usage,
  cpuLimit,
  memoryMB,
  diskMB,
  elapsed,
}: {
  usage: ResourceUsage | undefined;
  cpuLimit: number;
  memoryMB: number;
  diskMB: number;
  elapsed: number;
}) {
  const cpu = usage?.cpu_percent;
  return (
    <div className="resource-readings">
      <Reading
        label="CPU"
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
      <Reading
        label="Memory"
        value={usage?.memory_bytes ?? null}
        allocation={memoryMB * 1048576}
        fresh={containerFresh(usage, elapsed)}
        detail={
          usage !== undefined && usage.container_error.length > 0
            ? usage.container_error
            : "Container memory use, excluding reclaimable file cache."
        }
      />
      <Reading
        label="Game data"
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
