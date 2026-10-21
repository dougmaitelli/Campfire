import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { ResourceMeter } from "./ResourceMeter";
import { ResourceAllocation } from "./ResourceAllocation";
import { ResourceSparkline, type SparklineHistory } from "./ResourceSparkline";
import { resourceUtilization } from "./utilization";
import { EMPTY_VALUE } from "../../utils/formatDisplayValue";
import "./resource-reading.css";

export function ResourceReading({
  label,
  value,
  allocation,
  fresh = true,
  detail,
  format,
  formatUsed = format,
  formatAllocation = format,
  unit,
  history,
  icon,
  showLabel = true,
  showAllocation = true,
}: {
  label: string;
  value: number | null;
  allocation: number;
  fresh?: boolean;
  detail?: string;
  format: (value: number) => string;
  formatUsed?: (value: number) => string;
  formatAllocation?: (value: number) => string;
  unit?: string;
  history?: SparklineHistory | undefined;
  icon?: ReactNode;
  showLabel?: boolean;
  showAllocation?: boolean;
}) {
  const { t } = useTranslation();
  const measured = value !== null;
  const { tone, title } = resourceUtilization(fresh ? value : null, allocation);
  return (
    <div
      className={`usage-reading usage-reading-${tone}${history ? " usage-reading-history" : ""}${icon ? " usage-reading-icon" : ""}${measured && !fresh ? " usage-stale" : ""}`}
      title={detail ?? title}
    >
      {icon && <span className="usage-icon">{icon}</span>}
      <span className={showLabel ? "usage-label" : "visually-hidden"}>
        {label}
      </span>
      <span className="usage-value">
        {measured ? formatUsed(value) : EMPTY_VALUE}
        {showAllocation && (
          <ResourceAllocation>
            {allocation > 0
              ? formatAllocation(allocation)
              : t("resourceReadings.noLimit")}
            {allocation > 0 && unit && ` ${unit}`}
          </ResourceAllocation>
        )}
        {measured && !fresh && (
          <span className="usage-age"> {t("resourceReadings.stale")}</span>
        )}
      </span>
      {history && (
        <ResourceSparkline history={history} label={label} format={format} />
      )}
      {!measured ? (
        <div
          className="resource-meter resource-meter-placeholder"
          aria-hidden="true"
        />
      ) : (
        <ResourceMeter
          label={label}
          used={fresh ? value : null}
          allocated={allocation}
        />
      )}
      {tone !== "neutral" && <span className="visually-hidden">{title}</span>}
    </div>
  );
}
