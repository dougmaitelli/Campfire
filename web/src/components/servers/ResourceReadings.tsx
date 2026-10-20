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
  const { t } = useTranslation();
  const cpu = usage?.cpu_percent;
  return (
    <div
      className={`resource-readings${layout !== "stack" ? ` resource-readings-${layout}` : ""}`}
    >
      <ResourceReading
        label={t("common.cpu")}
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
