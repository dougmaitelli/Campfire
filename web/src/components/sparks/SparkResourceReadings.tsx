import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import {
  bytes,
  containerFresh,
  diskFresh,
  type MonitoredServer,
} from "../../api/monitoring";
import type { Spark } from "../../api/sparks";
import { ResourceReading } from "../servers/ResourceReadings";
import { cpuUsageDivisor, formatCPUUsage } from "../../utils/cpuUsage";

export function SparkResourceReadings({
  spark,
  servers,
  elapsed,
}: {
  spark: Spark;
  servers: MonitoredServer[] | undefined;
  elapsed: number;
}) {
  const { t } = useTranslation();
  const hosted = servers?.filter((server) => server.spark_id === spark.id);
  const readings = [
    {
      label: t("common.cpu"),
      key: "cpu_percent" as const,
      allocation: spark.cpu_limit > 0 ? 100 : 0,
      showAllocation: spark.cpu_limit <= 0,
      scale: 1 / cpuUsageDivisor(spark.cpu_limit),
      format: (value: number) => formatCPUUsage(value, spark.cpu_limit),
      fresh: containerFresh,
    },
    {
      label: t("common.memory"),
      key: "memory_bytes" as const,
      allocation: spark.memory_mb * 1048576,
      scale: 1,
      format: bytes,
      fresh: containerFresh,
    },
    {
      label: t("sparkResourceReadings.gameData"),
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
            showAllocation={reading.showAllocation ?? true}
            fresh={
              measured &&
              hosted.every((server) => reading.fresh(server.usage, elapsed))
            }
            detail={i18n.t(
              "sparkResourceReadings.combinedGameServerUsageAgainstThisSparkSConfiguredLimit",
            )}
            compact
            format={reading.format}
          />
        );
      })}
    </div>
  );
}
