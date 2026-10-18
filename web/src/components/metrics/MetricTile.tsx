import type { ReactNode } from "react";
import { EMPTY_VALUE } from "../../utils/formatDisplayValue";
import {
  resourceUtilization,
  utilizationThresholds,
  type UtilizationTone,
} from "./utilization";
import "./metric-tile.css";
import { ResourceMeter } from "./ResourceMeter";

export function MetricTile({
  label,
  value,
  detail,
  tone = "neutral",
  title,
  announcement,
  children,
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
  tone?: UtilizationTone;
  title?: string;
  announcement?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <article className={`metric-tile metric-tile-${tone}`} title={title}>
      <span className="metric-tile-label">{label}</span>
      <strong className="metric-tile-value">{value}</strong>
      {detail && <small className="metric-tile-detail">{detail}</small>}
      {children && <div className="metric-tile-visual">{children}</div>}
      {announcement && <span className="visually-hidden">{announcement}</span>}
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
  const { ratio, tone, title } = resourceUtilization(
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
      announcement={tone !== "neutral" ? title : undefined}
      detail={
        ratio === null
          ? "Utilization unavailable"
          : `${Math.round(ratio * 100)}% of allocated capacity`
      }
      value={
        <>
          {used === null ? EMPTY_VALUE : format(used)} / {format(allocated)}
          {unit && ` ${unit}`}
        </>
      }
    >
      {ratio !== null && (
        <ResourceMeter
          label={label}
          used={used}
          allocated={allocated}
          warningAt={warningAt}
          criticalAt={criticalAt}
        />
      )}
    </MetricTile>
  );
}
