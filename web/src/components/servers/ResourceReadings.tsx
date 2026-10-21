import { useTranslation } from "react-i18next";
import { NavigationIcon } from "../navigation/NavigationIcon";
import {
  bytes,
  containerFresh,
  diskFresh,
  type ResourceUsage,
} from "../../api/monitoring";
import "./resource-readings.css";
import { cpuUsageDivisor, formatCPUUsage } from "../../utils/cpuUsage";
import type { ResourceHistory } from "../../api/resourceHistory";
import type { SparklineHistory } from "../metrics/ResourceSparkline";
import { ResourceReading } from "../metrics/ResourceReading";
import { MetricTiles } from "../metrics/MetricTiles";

export function ResourceReadings({
  usage,
  cpuLimit,
  memoryMB,
  diskMB,
  elapsed,
  layout = "stack",
  history,
}: {
  usage: ResourceUsage | undefined;
  cpuLimit: number;
  memoryMB: number;
  diskMB: number;
  elapsed: number;
  layout?: "stack" | "grid" | "compact" | "columns";
  history?: ResourceHistory | undefined;
}) {
  const { t } = useTranslation();
  const cpu = usage?.cpu_percent;
  const series = (
    metric: "cpu_percent" | "memory_bytes" | "disk_bytes",
    divisor = 1,
  ): SparklineHistory | undefined =>
    history
      ? {
          from: history.from,
          to: history.to,
          interval: history.interval_seconds,
          points: history.samples.map((sample) => ({
            time: sample.collected_at,
            value:
              sample.usage[metric] === null
                ? null
                : sample.usage[metric] / divisor,
          })),
        }
      : undefined;
  const readings = [
    <ResourceReading
      key="cpu"
      icon={layout === "stack" ? <NavigationIcon name="cpu" /> : undefined}
      showLabel={layout !== "columns"}
      showAllocation={layout === "stack" || cpuLimit <= 0}
      label={t("common.cpu")}
      history={series("cpu_percent", cpuUsageDivisor(cpuLimit))}
      format={(value) => formatCPUUsage(value, cpuLimit)}
      formatAllocation={() => cpuLimit.toLocaleString()}
      unit="cores"
      value={
        cpu === undefined || cpu === null
          ? null
          : cpu / cpuUsageDivisor(cpuLimit)
      }
      allocation={cpuLimit > 0 ? 100 : 0}
      fresh={containerFresh(usage, elapsed)}
      detail={
        usage !== undefined && usage.container_error.length > 0
          ? usage.container_error
          : cpu === null
            ? t("resourceReadings.waitingForACPUSample")
            : t(
                cpuLimit > 0
                  ? "resourceReadings.cpuShareOfAllocatedCores"
                  : "resourceReadings.cpuCoresUsed",
              )
      }
    />,
    <ResourceReading
      key="memory"
      icon={layout === "stack" ? <NavigationIcon name="memory" /> : undefined}
      showLabel={layout !== "columns"}
      label={t("common.memory")}
      history={series("memory_bytes")}
      format={bytes}
      value={usage?.memory_bytes ?? null}
      allocation={memoryMB * 1048576}
      fresh={containerFresh(usage, elapsed)}
      detail={
        usage !== undefined && usage.container_error.length > 0
          ? usage.container_error
          : t(
              "resourceReadings.containerMemoryUseExcludingReclaimableFileCache",
            )
      }
    />,
    <ResourceReading
      key="disk"
      icon={layout === "stack" ? <NavigationIcon name="disk" /> : undefined}
      showLabel={layout !== "columns"}
      label={t("resourceReadings.gameData")}
      history={series("disk_bytes")}
      format={bytes}
      value={usage?.disk_bytes ?? null}
      allocation={diskMB * 1048576}
      fresh={diskFresh(usage, elapsed)}
      detail={
        usage !== undefined && usage.disk_error.length > 0
          ? usage.disk_error
          : usage?.disk_sampled_at
            ? t("resourceReadings.sampledBackupsAndImagesExcluded", {
                newDateToLocaleString: new Date(
                  usage.disk_sampled_at * 1000,
                ).toLocaleString(),
              })
            : t("resourceReadings.waitingForAGameDataSizeSample")
      }
    />,
  ];
  if (layout === "columns")
    return (
      <>
        {readings.map((reading) => (
          <td key={reading.key}>{reading}</td>
        ))}
      </>
    );
  if (layout === "stack") return <MetricTiles>{readings}</MetricTiles>;
  return (
    <div className={`resource-readings resource-readings-${layout}`}>
      {readings}
    </div>
  );
}
