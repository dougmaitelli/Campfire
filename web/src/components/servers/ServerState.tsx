import { StatusPill } from "../status/StatusPill";

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
  const tone =
    state === "running"
      ? "positive"
      : state === "failed" || state === "error" || state === "dead"
        ? "negative"
        : [
              "starting",
              "stopping",
              "restarting",
              "installing",
              "reinstalling",
              "updating",
              "restoring",
              "backing_up",
              "importing",
              "exporting",
              "deleting",
            ].includes(state)
          ? "pending"
          : "neutral";
  return <StatusPill tone={tone}>{label}</StatusPill>;
}
