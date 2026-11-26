import { useTranslation } from "react-i18next";
import type { ComponentProps } from "react";
import {
  containerFresh,
  diskFresh,
  type ResourceUsage,
} from "../../api/monitoring";
import { formatBytes, BYTES_PER_MEBIBYTE } from "../../utils/formatBytes";
import { cpuUsageDivisor, formatCPUUsage } from "../../utils/cpuUsage";
import type { ResourceReading } from "./ResourceReading";
import { DiskAccounting } from "../../contracts/domains";

const diskDescriptions = {
  [DiskAccounting.Project]: "resourceReadings.projectDiskSample",
  [DiskAccounting.Qgroup]: "resourceReadings.qgroupDiskSample",
  [DiskAccounting.Logical]: "resourceReadings.logicalDiskSample",
  [DiskAccounting.Mixed]: "resourceReadings.mixedDiskSample",
} as const;

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
  const diskDescription = usage?.disk_sampled_at
    ? t(diskDescriptions[usage.disk_accounting ?? DiskAccounting.Logical], {
        newDateToLocaleString: new Date(
          usage.disk_sampled_at * 1000,
        ).toLocaleString(),
      })
    : t("resourceReadings.waitingForAGameDataSizeSample");
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
      format: formatBytes,
      value: usage?.memory_bytes ?? null,
      allocation: memoryMB * BYTES_PER_MEBIBYTE,
      fresh: containerFresh(usage, elapsed),
      detail:
        detail ??
        containerError ??
        t("resourceReadings.containerMemoryUseExcludingReclaimableFileCache"),
    },
    {
      key: "disk",
      label: t("resourceReadings.gameData"),
      format: formatBytes,
      value: usage?.disk_bytes ?? null,
      allocation: diskMB * BYTES_PER_MEBIBYTE,
      fresh: diskFresh(usage, elapsed),
      detail: [
        usage?.disk_error,
        usage?.disk_quota_error,
        detail,
        diskDescription,
      ]
        .filter(Boolean)
        .join(" "),
    },
  ] satisfies (ComponentProps<typeof ResourceReading> & { key: string })[];
}
