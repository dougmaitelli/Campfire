import { StatusPill, type StatusTone } from "../status/StatusPill";
import { useStatusLabel } from "../status/useStatusLabel";

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
  const label = useStatusLabel(outcome);
  return <StatusPill tone={tones[outcome] ?? "neutral"}>{label}</StatusPill>;
}
