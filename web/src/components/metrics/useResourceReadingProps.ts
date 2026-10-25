import { useTranslation } from "react-i18next";
import type { ComponentProps } from "react";
import {
  bytes,
  containerFresh,
  diskFresh,
  type ResourceUsage,
} from "../../api/monitoring";
import { cpuUsageDivisor, formatCPUUsage } from "../../utils/cpuUsage";
import type { ResourceReading } from "./ResourceReading";

export interface ResourceReadingOptions {
  usage: ResourceUsage | undefined;
  cpuLimit: number;
  memoryMB: number;
  diskMB: number;
  elapsed: number;
  detail?: string;
}

export function useResourceReadingProps({
  usage,
  cpuLimit,
  memoryMB,
  diskMB,
  elapsed,
  detail,
}: ResourceReadingOptions) {
  const { t } = useTranslation();
  const cpu = usage?.cpu_percent;
  const containerError = usage?.container_error.length
    ? usage.container_error
    : undefined;
  const diskError = usage?.disk_error.length ? usage.disk_error : undefined;
  return [
    {
      key: "cpu",
      label: t("common.cpu"),
      showAllocation: cpuLimit <= 0,
      format: (value: number) => formatCPUUsage(value, cpuLimit),
      formatAllocation: () => cpuLimit.toLocaleString(),
      unit: "cores",
      value:
        cpu === undefined || cpu === null
          ? null
          : cpu / cpuUsageDivisor(cpuLimit),
      allocation: cpuLimit > 0 ? 100 : 0,
      fresh: containerFresh(usage, elapsed),
      detail:
        detail ??
        containerError ??
        (cpu === null
          ? t("resourceReadings.waitingForACPUSample")
          : t(
              cpuLimit > 0
                ? "resourceReadings.cpuShareOfAllocatedCores"
                : "resourceReadings.cpuCoresUsed",
            )),
    },
    {
      key: "memory",
      label: t("common.memory"),
      format: bytes,
      value: usage?.memory_bytes ?? null,
      allocation: memoryMB * 1048576,
      fresh: containerFresh(usage, elapsed),
      detail:
        detail ??
        containerError ??
        t("resourceReadings.containerMemoryUseExcludingReclaimableFileCache"),
    },
    {
      key: "disk",
      label: t("resourceReadings.gameData"),
      format: bytes,
      value: usage?.disk_bytes ?? null,
      allocation: diskMB * 1048576,
      fresh: diskFresh(usage, elapsed),
      detail:
        detail ??
        diskError ??
        (usage?.disk_sampled_at
          ? t("resourceReadings.sampledBackupsAndImagesExcluded", {
              newDateToLocaleString: new Date(
                usage.disk_sampled_at * 1000,
              ).toLocaleString(),
            })
          : t("resourceReadings.waitingForAGameDataSizeSample")),
    },
  ] satisfies (ComponentProps<typeof ResourceReading> & { key: string })[];
}
