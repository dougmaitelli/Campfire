import { resourceUtilization, utilizationThresholds } from "./utilization";
import "./resource-meter.css";

export function ResourceMeter({
  label,
  used,
  allocated,
  warningAt = utilizationThresholds.warning,
  criticalAt = utilizationThresholds.critical,
}: {
  label: string;
  used: number | null;
  allocated: number;
  warningAt?: number;
  criticalAt?: number;
}) {
  const { ratio, tone, title } = resourceUtilization(
    used,
    allocated,
    warningAt,
    criticalAt,
  );
  if (ratio === null) return null;
  return (
    <meter
      className={`resource-meter resource-meter-${tone}`}
      min={0}
      max={1}
      low={warningAt}
      high={criticalAt}
      optimum={0}
      value={Math.min(ratio, 1)}
      aria-label={`${label} usage against allocation`}
      aria-valuetext={title}
      title={title}
    />
  );
}
