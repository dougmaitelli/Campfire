import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import { StatusPill, type StatusTone } from "../status/StatusPill";
import "./server-state.css";

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
  get running() {
    return i18n.t("common.running");
  },
  get stopped() {
    return i18n.t("common.stopped");
  },
  get not_installed() {
    return i18n.t("serverState.notInstalled");
  },
  get deleted() {
    return i18n.t("serverState.deleted");
  },
  get deleting() {
    return i18n.t("serverState.deleting");
  },
  get paused() {
    return i18n.t("serverState.paused");
  },
  get created() {
    return i18n.t("serverState.created");
  },
  get exited() {
    return i18n.t("common.stopped");
  },
  get dead() {
    return i18n.t("common.failed");
  },
  get unknown() {
    return i18n.t("common.unavailable");
  },
  get starting() {
    return i18n.t("serverState.starting");
  },
  get stopping() {
    return i18n.t("serverState.stopping");
  },
  get restarting() {
    return i18n.t("serverState.restarting");
  },
  get installing() {
    return i18n.t("serverState.installing");
  },
  get reinstalling() {
    return i18n.t("serverState.reinstalling");
  },
  get updating() {
    return i18n.t("serverState.updating");
  },
  get restoring() {
    return i18n.t("serverState.restoringBackup");
  },
  get backing_up() {
    return i18n.t("serverState.creatingBackup");
  },
  get importing() {
    return i18n.t("serverState.importing");
  },
  get exporting() {
    return i18n.t("serverState.exporting");
  },
  get failed() {
    return i18n.t("common.failed");
  },
  get error() {
    return i18n.t("serverState.error");
  },
};

export function ServerState({
  state: runtimeState = "unknown",
  ready = true,
  startedAt,
  stoppedAt,
  now = 0,
}: {
  state?: string;
  ready?: boolean;
  startedAt?: number | undefined;
  stoppedAt?: number | undefined;
  now?: number;
}) {
  const { t } = useTranslation();
  const state =
    runtimeState === "running" && !ready ? "starting" : runtimeState;
  const label = labels[state] ?? t("common.unavailable");
  const since =
    state === "running"
      ? startedAt
      : ["stopped", "exited"].includes(state)
        ? stoppedAt
        : undefined;
  let detail: string | undefined;
  if (since && since > 0 && since <= now) {
    const seconds = Math.floor(now - since);
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const duration = days
      ? t("serverState.durationDays", { days, hours })
      : hours
        ? t("serverState.durationHours", { hours, minutes })
        : minutes
          ? t("serverState.durationMinutes", { minutes })
          : t("serverState.durationSeconds", { seconds });
    detail = t(
      state === "running" ? "serverState.uptime" : "serverState.stoppedAgo",
      { duration },
    );
  }
  return (
    <span className="server-state">
      <StatusPill
        tone={labels[state] ? (tones[state] ?? "neutral") : "negative"}
      >
        {label}
      </StatusPill>
      {detail && (
        <span
          className="server-state-timing"
          title={new Date(since! * 1000).toLocaleString()}
        >
          {detail}
        </span>
      )}
    </span>
  );
}
