import { useTranslation } from "react-i18next";
import {
  bytes,
  containerFresh,
  diskFresh,
  type ResourceUsage,
} from "../../api/monitoring";
import "./resource-readings.css";
import { ResourceMeter } from "../metrics/ResourceMeter";
import { EMPTY_VALUE } from "../../utils/formatDisplayValue";
import type { ResourceHistory } from "../../api/resourceHistory";
import {
  ResourceSparkline,
  type SparklineHistory,
} from "../metrics/ResourceSparkline";

export function ResourceReading({
  label,
  value,
  allocation,
  fresh,
  detail,
  compact,
  format,
  history,
}: {
  label: string;
  value: number | null;
  allocation: number;
  fresh: boolean;
  detail: string;
  compact: boolean;
  format: (value: number) => string;
  history?: SparklineHistory | undefined;
}) {
  const { t } = useTranslation();
  const measured = value !== null;
  return (
    <div
      className={`usage-reading${measured && !fresh ? " usage-stale" : ""}`}
      title={detail}
    >
      <span className="usage-label">{label}</span>
      <span>
        {measured ? format(value) : EMPTY_VALUE}
        <span className="usage-allocation">
          {measured ? " / " : "/"}
          {allocation > 0 ? format(allocation) : t("resourceReadings.noLimit")}
          {compact || allocation <= 0 ? "" : " allocated"}
        </span>
        {measured && !fresh && (
          <span className="usage-age"> {t("resourceReadings.stale")}</span>
        )}
      </span>
      {history && (
        <ResourceSparkline history={history} label={label} format={format} />
      )}
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
  history,
}: {
  usage: ResourceUsage | undefined;
  cpuLimit: number;
  memoryMB: number;
  diskMB: number;
  elapsed: number;
  layout?: "stack" | "grid" | "compact";
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
  return (
    <div
      className={`resource-readings${layout !== "stack" ? ` resource-readings-${layout}` : ""}`}
    >
      <ResourceReading
        label={t("common.cpu")}
        history={series("cpu_percent", 100)}
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
              ? t("resourceReadings.waitingForACPUSample")
              : t("resourceReadings.measuredCPUUseOneFullyUsedCoreIs100")
        }
      />
      <ResourceReading
        label={t("common.memory")}
        history={series("memory_bytes")}
        format={bytes}
        compact={layout !== "stack"}
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
      />
      <ResourceReading
        label={t("resourceReadings.gameData")}
        history={series("disk_bytes")}
        format={bytes}
        compact={layout !== "stack"}
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
      />
    </div>
  );
}
