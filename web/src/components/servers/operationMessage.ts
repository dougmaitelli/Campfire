import i18n from "../../i18n";
import type { RuntimeStatus } from "../../api/runtime";
import { formatErrorMessage } from "../../utils/errors";

const operationNames: Record<string, string> = {
  get command() {
    return i18n.t("operationMessage.consoleCommand");
  },
  get install() {
    return i18n.t("operationMessage.installation");
  },
  get update() {
    return i18n.t("operationMessage.gameFileUpdate");
  },
  get reinstall() {
    return i18n.t("operationMessage.reinstallation");
  },
  get start() {
    return i18n.t("operationMessage.serverStart");
  },
  get stop() {
    return i18n.t("operationMessage.serverStop");
  },
  get restart() {
    return i18n.t("operationMessage.serverRestart");
  },
  get kill() {
    return i18n.t("operationMessage.forcedStop");
  },
  get backup() {
    return i18n.t("operationMessage.backup");
  },
  get restore() {
    return i18n.t("operationMessage.backupRestoration");
  },
  get delete() {
    return i18n.t("operationMessage.serverDeletion");
  },
  get transfer() {
    return i18n.t("operationMessage.serverTransfer");
  },
};

export function operationLabel(operation: string) {
  return (
    operationNames[operation] ?? i18n.t("operationMessage.serverOperation")
  );
}

export function operationMessage(
  status:
    | Pick<
        RuntimeStatus,
        | "operation"
        | "operation_id"
        | "operation_state"
        | "progress"
        | "error"
        | "error_code"
        | "busy"
      >
    | undefined,
  currentOperationID: string | null = null,
): {
  title: string;
  detail?: string;
  tone: "error" | "info" | "success";
} | null {
  if (
    !status?.operation_state &&
    !status?.error_code &&
    !status?.error &&
    !status?.progress
  )
    return null;
  const name = operationLabel(status.operation ?? "");
  if (
    status.operation_state === "interrupted" ||
    status.error_code === "operation_interrupted"
  )
    return {
      title: i18n.t("operationMessage.interrupted", { name: name }),
      ...(status.error ? { detail: formatErrorMessage(status.error) } : {}),
      tone: "error",
    };
  if (
    status.error_code ||
    status.error ||
    status.operation_state === "failed"
  ) {
    return {
      title: i18n.t("operationMessage.failed", { name: name }),
      ...(status.error ? { detail: formatErrorMessage(status.error) } : {}),
      tone: "error",
    };
  }
  if (status.operation_state === "succeeded")
    return currentOperationID && status.operation_id === currentOperationID
      ? {
          title: i18n.t("operationMessage.completed", { name: name }),
          tone: "success",
        }
      : null;
  if (status.operation_state === "queued")
    return {
      title: i18n.t("operationMessage.queued", { name: name }),
      tone: "info",
    };
  return {
    title:
      status.operation && (status.busy || status.operation_state === "running")
        ? i18n.t("operationMessage.inProgress", { name: name })
        : i18n.t("operationMessage.serverStatus"),
    ...(status.progress ? { detail: formatErrorMessage(status.progress) } : {}),
    tone: "info",
  };
}
