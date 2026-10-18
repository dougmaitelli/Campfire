import { StatusPill, type StatusTone } from "../status/StatusPill";

const tones: Record<string, StatusTone> = {
  success: "positive",
  succeeded: "positive",
  completed: "positive",
  failed: "negative",
  error: "negative",
  denied: "negative",
  pending: "pending",
  queued: "pending",
  accepted: "pending",
  running: "positive",
  interrupted: "warning",
  skipped: "warning",
};

export function AuditOutcome({ outcome }: { outcome: string }) {
  return (
    <StatusPill tone={tones[outcome] ?? "neutral"}>
      {outcome
        .replaceAll("_", " ")
        .replace(/^./, (letter) => letter.toUpperCase())}
    </StatusPill>
  );
}
