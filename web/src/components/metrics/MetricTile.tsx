import type { ReactNode } from "react";
import { EMPTY_VALUE } from "../../utils/formatDisplayValue";
import {
  resourceUtilization,
  utilizationThresholds,
  type UtilizationTone,
} from "./utilization";
import "./metric-tile.css";
import { ResourceMeter } from "./ResourceMeter";
import { useTranslation } from "react-i18next";

export function MetricTile({
  label,
  value,
  detail,
  tone = "neutral",
  title,
  announcement,
  children,
  icon,
}: {
  label: string;
  value: ReactNode;
  detail?: ReactNode;
  tone?: UtilizationTone;
  title?: string;
  announcement?: ReactNode;
  children?: ReactNode;
  icon?: ReactNode;
}) {
  return (
    <article className={`metric-tile metric-tile-${tone}`} title={title}>
      {icon && <span className="metric-tile-icon">{icon}</span>}
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
  icon,
  formatUsed = format,
  warningAt = utilizationThresholds.warning,
  criticalAt = utilizationThresholds.critical,
}: {
  label: string;
  used: number | null;
  allocated: number;
  format: (value: number) => string;
  unit?: string;
  icon?: ReactNode;
  formatUsed?: (value: number) => string;
  warningAt?: number;
  criticalAt?: number;
}) {
  const { t } = useTranslation();
  const { ratio, tone, title } = resourceUtilization(
    used,
    allocated,
    warningAt,
    criticalAt,
  );
  return (
    <MetricTile
      label={label}
      icon={icon}
      tone={tone}
      title={title}
      announcement={tone !== "neutral" ? title : undefined}
      value={
        <>
          {used === null ? EMPTY_VALUE : formatUsed(used)}
          <span className="metric-tile-allocation">
            {" / "}
            {allocated > 0 ? format(allocated) : t("resourceReadings.noLimit")}
            {allocated > 0 && unit && ` ${unit}`}
          </span>
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
