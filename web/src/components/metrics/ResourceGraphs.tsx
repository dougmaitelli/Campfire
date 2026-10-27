import { useTranslation } from "react-i18next";
import { bytes } from "../../api/monitoring";
import { useResourceHistory } from "../../hooks/useResourceHistory";
import { cpuUsageDivisor, formatCPUUsage } from "../../utils/cpuUsage";
import { InlineMessage } from "../status/InlineMessage";
import { ResourceGraph, type GraphHistory } from "./ResourceGraph";

export function ResourceGraphs({
  serverID,
  cpuLimit,
  memoryMB,
}: {
  serverID: string;
  cpuLimit: number;
  memoryMB: number;
}) {
  const { t } = useTranslation();
  const result = useResourceHistory(serverID);
  const history = result?.history;
  const series = (
    metric: "cpu_percent" | "memory_bytes",
  ): GraphHistory | undefined =>
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
                : sample.usage[metric] /
                  (metric === "cpu_percent" ? cpuUsageDivisor(cpuLimit) : 1),
          })),
        }
      : undefined;
  return (
    <>
      {result?.error && (
        <InlineMessage tone="info" role="status">
          {result.error}
        </InlineMessage>
      )}
      <div className="resource-graphs">
        <ResourceGraph
          label={t("common.cpu")}
          history={series("cpu_percent")}
          format={(value) => formatCPUUsage(value, cpuLimit)}
          allocation={cpuLimit > 0 ? 100 : 0}
          loading={!result}
        />
        <ResourceGraph
          label={t("common.memory")}
          history={series("memory_bytes")}
          format={bytes}
          allocation={memoryMB * 1048576}
          loading={!result}
        />
      </div>
    </>
  );
}
