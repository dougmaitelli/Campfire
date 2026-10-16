import type { ReactNode } from "react";
import "./status-pill.css";

export function StatusPill({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "neutral" | "positive" | "negative" | "pending";
}) {
  return <span className={`status-pill status-pill-${tone}`}>{children}</span>;
}
