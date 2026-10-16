import type { ReactNode } from "react";
import "./status-pill.css";

type StatusTone = "neutral" | "positive" | "negative" | "pending" | "warning";

const tones: Record<string, StatusTone> = {
  running: "positive",
  enabled: "positive",
  connected: "positive",
  online: "positive",
  succeeded: "positive",
  completed: "positive",
  available: "positive",
  syncing: "pending",
  success: "positive",
  failed: "negative",
  error: "negative",
  dead: "negative",
  offline: "negative",
  disconnected: "negative",
  unavailable: "negative",
  unknown: "negative",
  denied: "negative",
  disabled: "warning",
  paused: "warning",
  interrupted: "warning",
  skipped: "warning",
  not_installed: "warning",
  pending: "pending",
  queued: "pending",
  accepted: "pending",
  starting: "pending",
  stopping: "pending",
  restarting: "pending",
  installing: "pending",
  reinstalling: "pending",
  updating: "pending",
  restoring: "pending",
  backing_up: "pending",
  importing: "pending",
  exporting: "pending",
  deleting: "pending",
};

export function StatusPill({
  children,
  tone,
  status,
}: {
  children?: ReactNode;
  tone?: StatusTone | undefined;
  status?: string;
}) {
  const selectedTone = tone ?? tones[status ?? ""] ?? "neutral";
  const label = status
    ? status
        .replace(/[_-]/g, " ")
        .replace(/^./, (letter) => letter.toUpperCase())
    : "";
  return (
    <span className={`status-pill status-pill-${selectedTone}`}>
      {children ?? label}
    </span>
  );
}
