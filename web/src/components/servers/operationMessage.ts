import type { RuntimeStatus } from "../../api/runtime";
import { formatErrorMessage } from "../../utils/errors";

const operationNames: Record<string, string> = {
  install: "Installation",
  update: "Game file update",
  reinstall: "Reinstallation",
  start: "Server start",
  stop: "Server stop",
  restart: "Server restart",
  kill: "Forced stop",
  backup: "Backup",
  restore: "Backup restoration",
  delete: "Server deletion",
  transfer: "Server transfer",
};

export function operationMessage(
  status:
    | Pick<
        RuntimeStatus,
        "operation" | "operation_state" | "progress" | "error" | "error_code"
      >
    | undefined,
): {
  title: string;
  detail?: string;
  tone: "error" | "info" | "success";
} | null {
  if (!status?.operation_state && !status?.error_code && !status?.progress)
    return null;
  const name = operationNames[status.operation ?? ""] ?? "Server operation";
  if (
    status.operation_state === "interrupted" ||
    status.error_code === "operation_interrupted"
  )
    return {
      title: `${name} interrupted.`,
      ...(status.error ? { detail: formatErrorMessage(status.error) } : {}),
      tone: "error",
    };
  if (status.error_code || status.operation_state === "failed") {
    return {
      title: `${name} failed.`,
      ...(status.error ? { detail: formatErrorMessage(status.error) } : {}),
      tone: "error",
    };
  }
  if (status.operation_state === "succeeded")
    return { title: `${name} completed.`, tone: "success" };
  if (status.operation_state === "queued")
    return { title: `${name} queued.`, tone: "info" };
  return {
    title: status.operation ? `${name} in progress.` : "Server status.",
    ...(status.progress ? { detail: formatErrorMessage(status.progress) } : {}),
    tone: "info",
  };
}
