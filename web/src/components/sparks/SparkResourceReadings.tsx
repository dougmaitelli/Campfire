import {
  bytes,
  containerFresh,
  diskFresh,
  type MonitoredServer,
} from "../../api/monitoring";
import type { Spark } from "../../api/sparks";
import { ResourceReading } from "../servers/ResourceReadings";

export function SparkResourceReadings({
  spark,
  servers,
  elapsed,
}: {
  spark: Spark;
  servers: MonitoredServer[] | undefined;
  elapsed: number;
}) {
  const hosted = servers?.filter((server) => server.spark_id === spark.id);
  const readings = [
    {
      label: "CPU",
      key: "cpu_percent" as const,
      allocation: spark.cpu_limit,
      scale: 0.01,
      format: (value: number) =>
        `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })} cores`,
      fresh: containerFresh,
    },
    {
      label: "Memory",
      key: "memory_bytes" as const,
      allocation: spark.memory_mb * 1048576,
      scale: 1,
      format: bytes,
      fresh: containerFresh,
    },
    {
      label: "Game data",
      key: "disk_bytes" as const,
      allocation: spark.disk_mb * 1048576,
      scale: 1,
      format: bytes,
      fresh: diskFresh,
    },
  ];
  return (
    <div className="resource-readings resource-readings-compact">
      {readings.map((reading) => {
        const measured =
          spark.connected &&
          hosted !== undefined &&
          hosted.every((server) => server.usage[reading.key] !== null);
        const value = measured
          ? hosted.reduce(
              (sum, server) =>
                sum + (server.usage[reading.key] ?? 0) * reading.scale,
              0,
            )
          : null;
        return (
          <ResourceReading
            key={reading.key}
            label={reading.label}
            value={value}
            allocation={reading.allocation}
            fresh={
              measured &&
              hosted.every((server) => reading.fresh(server.usage, elapsed))
            }
            detail="Combined game-server usage against this Spark's configured limit. Excludes host processes, backups, and container images."
            compact
            format={reading.format}
          />
        );
      })}
    </div>
  );
}
