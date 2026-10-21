import type { ReactNode } from "react";
import "./metric-tiles.css";

export function MetricTiles({ children }: { children: ReactNode }) {
  return <div className="metric-tiles">{children}</div>;
}
