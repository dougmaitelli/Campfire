import type { ResourceHistory } from "../../api/resourceHistory";
import { cpuUsageDivisor } from "../../utils/cpuUsage";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { MetricTiles } from "./MetricTiles";
import { ResourceReading } from "./ResourceReading";
import {
  useResourceReadingProps,
  type ResourceReadingOptions,
} from "./useResourceReadingProps";
import type { SparklineHistory } from "./ResourceSparkline";

export function ResourceTiles({
  history,
  ...options
}: ResourceReadingOptions & { history?: ResourceHistory | undefined }) {
  const readings = useResourceReadingProps(options);
  return (
    <MetricTiles>
      {readings.map(({ key, ...reading }) => {
        const metric =
          key === "cpu"
            ? "cpu_percent"
            : key === "memory"
              ? "memory_bytes"
              : "disk_bytes";
        const divisor = key === "cpu" ? cpuUsageDivisor(options.cpuLimit) : 1;
        const series: SparklineHistory | undefined = history
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
          <ResourceReading
            key={key}
            {...reading}
            showAllocation
            icon={
              <NavigationIcon
                name={
                  key === "cpu" ? "cpu" : key === "memory" ? "memory" : "disk"
                }
              />
            }
            history={series}
          />
        );
      })}
    </MetricTiles>
  );
}
