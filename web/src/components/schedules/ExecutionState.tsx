import { StatusPill, type StatusTone } from "../status/StatusPill";
import { useStatusLabel } from "../status/useStatusLabel";

const tones: Record<string, StatusTone> = {
  pending: "pending",
  queued: "pending",
  accepted: "pending",
  running: "pending",
  succeeded: "positive",
  completed: "positive",
  failed: "negative",
  interrupted: "warning",
  skipped: "warning",
};

export function ExecutionState({ state }: { state: string }) {
  const label = useStatusLabel(state);
  return <StatusPill tone={tones[state] ?? "neutral"}>{label}</StatusPill>;
}
