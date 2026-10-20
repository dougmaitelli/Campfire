import { useTranslation } from "react-i18next";
import "./resource-sparkline.css";

export interface SparklineHistory {
  from: number;
  to: number;
  interval: number;
  points: { time: number; value: number | null }[];
}

export function ResourceSparkline({
  history,
  label,
  format,
}: {
  history: SparklineHistory;
  label: string;
  format: (value: number) => string;
}) {
  const { t } = useTranslation();
  const maximum = Math.max(
    1,
    ...history.points.map((point) => point.value ?? 0),
  );
  const x = (time: number) =>
    Math.max(
      0,
      Math.min(
        300,
        ((time - history.from) / Math.max(1, history.to - history.from)) * 300,
      ),
    );
  const y = (value: number) => 29 - (value / maximum) * 26;
  const path = history.points
    .map((point, index) => {
      if (point.value === null) return "";
      const previous = history.points[index - 1];
      const command =
        previous &&
        previous.value !== null &&
        point.time - previous.time <= history.interval * 1.5
          ? "L"
          : "M";
      return `${command}${x(point.time)},${y(point.value)} l0.5,0`;
    })
    .join(" ");
  return (
    <svg
      className="resource-sparkline"
      viewBox="0 0 301 32"
      preserveAspectRatio="none"
      role="img"
      aria-label={t("resourceHistory.chartLabel", { resource: label })}
    >
      <path d={path} />
      {history.points.map((point) =>
        point.value === null ? null : (
          <circle key={point.time} cx={x(point.time)} cy={y(point.value)} r={3}>
            <title>{`${new Date(point.time * 1000).toLocaleString()}: ${format(point.value)}`}</title>
          </circle>
        ),
      )}
    </svg>
  );
}
