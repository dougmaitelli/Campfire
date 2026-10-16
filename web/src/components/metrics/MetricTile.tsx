import type { ReactNode } from "react";
import {
  resourceUtilization,
  utilizationThresholds,
  type UtilizationTone,
} from "./utilization";
import "./metric-tile.css";

export function MetricTile({
  label,
  value,
  detail,
  tone = "neutral",
  title,
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
  tone?: UtilizationTone;
  title?: string;
}) {
  return (
    <article className={`metric-tile metric-tile-${tone}`} title={title}>
      <span className="metric-tile-label">{label}</span>
      <strong className="metric-tile-value">{value}</strong>
      {detail && <small className="metric-tile-detail">{detail}</small>}
      {tone !== "neutral" && (
        <span className="visually-hidden">
          {tone === "critical" ? "Critical utilization" : "High utilization"}
        </span>
      )}
    </article>
  );
}

export function ResourceTile({
  label,
  used,
  allocated,
  format,
  unit,
  warningAt = utilizationThresholds.warning,
  criticalAt = utilizationThresholds.critical,
}: {
  label: string;
  used: number | null;
  allocated: number;
  format: (value: number) => string;
  unit?: string;
  warningAt?: number;
  criticalAt?: number;
}) {
  const { tone, title } = resourceUtilization(
    used,
    allocated,
    warningAt,
    criticalAt,
  );
  return (
    <MetricTile
      label={label}
      tone={tone}
      title={title}
      value={
        <>
          {used === null ? "-" : format(used)} / {format(allocated)}
          {unit && ` ${unit}`}
        </>
      }
    />
  );
}
