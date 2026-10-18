import { StatusPill, type StatusTone } from "../status/StatusPill";

const tones: Record<string, StatusTone> = {
  running: "positive",
  not_installed: "warning",
  paused: "warning",
  dead: "negative",
  unknown: "negative",
  failed: "negative",
  error: "negative",
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

const labels: Record<string, string> = {
  running: "Running",
  stopped: "Stopped",
  not_installed: "Not installed",
  deleted: "Deleted",
  deleting: "Deleting",
  paused: "Paused",
  created: "Created",
  exited: "Stopped",
  dead: "Failed",
  unknown: "Unavailable",
  starting: "Starting",
  stopping: "Stopping",
  restarting: "Restarting",
  installing: "Installing",
  reinstalling: "Reinstalling",
  updating: "Updating",
  restoring: "Restoring backup",
  backing_up: "Creating backup",
  importing: "Importing",
  exporting: "Exporting",
  failed: "Failed",
  error: "Error",
};

export function ServerState({
  state: runtimeState = "unknown",
  ready = true,
}: {
  state?: string;
  ready?: boolean;
}) {
  const state =
    runtimeState === "running" && !ready ? "starting" : runtimeState;
  const label = labels[state] ?? "Unavailable";
  return (
    <StatusPill tone={labels[state] ? (tones[state] ?? "neutral") : "negative"}>
      {label}
    </StatusPill>
  );
}
