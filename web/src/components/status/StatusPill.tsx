import type { ReactNode } from "react";
import "./status-pill.css";

export type StatusTone =
  "neutral" | "positive" | "negative" | "pending" | "warning";

export function StatusPill({
  children,
  tone = "neutral",
  indicator = true,
}: {
  children?: ReactNode;
  tone?: StatusTone | undefined;
  indicator?: boolean;
}) {
  return (
    <span
      className={`status-pill status-pill-${tone}${indicator ? "" : " status-pill-no-indicator"}`}
    >
      {children}
    </span>
  );
}
