import type { ReactNode } from "react";
import type { UtilizationTone } from "./utilization";
import "./metric-tile.css";

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
