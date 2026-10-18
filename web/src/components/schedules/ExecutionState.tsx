import { StatusPill, type StatusTone } from "../status/StatusPill";

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
  return (
    <StatusPill tone={tones[state] ?? "neutral"}>
      {state
        .replaceAll("_", " ")
        .replace(/^./, (letter) => letter.toUpperCase())}
    </StatusPill>
  );
}
