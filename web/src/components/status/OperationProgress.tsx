import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ProgressBar } from "./ProgressBar";
import "./work-progress.css";

/** A measurement, not an overall completion verdict. Workflow owners render
 * phases, cancellation controls and confirmed terminal outcomes separately. */
export function OperationProgress({
  label,
  value = 0,
  total,
  detail,
  active = true,
  stale = false,
}: {
  label: string;
  value?: number;
  total?: number | undefined;
  detail?: ReactNode;
  active?: boolean;
  stale?: boolean;
}) {
  const { t } = useTranslation();
  const known = total !== undefined && total > 0;
  const percent = known
    ? Math.min(100, Math.max(0, Math.floor((value / total) * 100)))
    : undefined;
  return (
    <span className="operation-progress">
      {active && !stale && (
        <ProgressBar
          aria-label={label}
          aria-valuetext={typeof detail === "string" ? detail : undefined}
          {...(known ? { value, max: total } : {})}
        />
      )}
      {stale && <span>{t("workProgress.stale")}</span>}
      <span className="operation-progress-metrics">
        <span>{detail ?? label}</span>
        {percent !== undefined && <strong>{percent}%</strong>}
      </span>
      {active && !stale && known && value >= total && (
        <span>{t("workProgress.unconfirmed")}</span>
      )}
    </span>
  );
}
