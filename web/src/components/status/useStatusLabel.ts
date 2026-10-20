import { useTranslation } from "react-i18next";

const labels = {
  pending: "common.pending",
  queued: "status.queued",
  accepted: "status.accepted",
  running: "common.running",
  success: "status.success",
  succeeded: "status.succeeded",
  completed: "status.completed",
  failed: "common.failed",
  error: "status.error",
  denied: "status.denied",
  interrupted: "status.interrupted",
  skipped: "status.skipped",
  cancelled: "status.cancelled",
} as const;

export function useStatusLabel(state: string) {
  const { t } = useTranslation();
  return t(
    Object.hasOwn(labels, state)
      ? labels[state as keyof typeof labels]
      : "common.unknown",
  );
}
