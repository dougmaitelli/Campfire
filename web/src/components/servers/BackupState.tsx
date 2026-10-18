import { StatusPill, type StatusTone } from "../status/StatusPill";

const states: Record<string, { label: string; tone: StatusTone }> = {
  available: { label: "Available", tone: "positive" },
  syncing: { label: "Syncing", tone: "pending" },
  deleting: { label: "Deleting", tone: "pending" },
  failed: { label: "Failed", tone: "negative" },
  error: { label: "Error", tone: "negative" },
  unavailable: { label: "Unavailable", tone: "negative" },
};

export function BackupState({ state }: { state: string }) {
  const display = states[state] ?? {
    label: state
      .replaceAll("_", " ")
      .replace(/^./, (letter) => letter.toUpperCase()),
    tone: "neutral",
  };
  return <StatusPill tone={display.tone}>{display.label}</StatusPill>;
}
