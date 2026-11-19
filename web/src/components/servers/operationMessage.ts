import {
  OperationState,
  ServerAction,
  RuntimeErrorCode,
} from "../../contracts/domains";
import i18n from "../../i18n";
import type { RuntimeStatus } from "../../api/runtime";
import { formatErrorMessage } from "../../utils/errors";

const operationNames: Partial<Record<ServerAction, string>> = {
  get [ServerAction.Command]() {
    return i18n.t("operationMessage.consoleCommand");
  },
  get [ServerAction.Install]() {
    return i18n.t("operationMessage.installation");
  },
  get [ServerAction.Update]() {
    return i18n.t("operationMessage.gameFileUpdate");
  },
  get [ServerAction.Reinstall]() {
    return i18n.t("operationMessage.reinstallation");
  },
  get [ServerAction.Start]() {
    return i18n.t("operationMessage.serverStart");
  },
  get [ServerAction.Stop]() {
    return i18n.t("operationMessage.serverStop");
  },
  get [ServerAction.Restart]() {
    return i18n.t("operationMessage.serverRestart");
  },
  get [ServerAction.Kill]() {
    return i18n.t("operationMessage.forcedStop");
  },
  get [ServerAction.Backup]() {
    return i18n.t("operationMessage.backup");
  },
  get [ServerAction.Restore]() {
    return i18n.t("operationMessage.backupRestoration");
  },
  get [ServerAction.Delete]() {
    return i18n.t("operationMessage.serverDeletion");
  },
  get [ServerAction.Transfer]() {
    return i18n.t("operationMessage.serverTransfer");
  },
};

export function operationLabel(operation?: ServerAction) {
  return (
    (operation ? operationNames[operation] : undefined) ??
    i18n.t("operationMessage.serverOperation")
  );
}

export function isCurrentOperationSuccessful(
  status:
    | Pick<
        RuntimeStatus,
        "operation_id" | "operation_state" | "busy" | "error" | "error_code"
      >
    | undefined,
  currentOperationID: string | null,
): boolean {
  return (
    !!currentOperationID &&
    status?.operation_id === currentOperationID &&
    status.operation_state === OperationState.Succeeded &&
    !status.busy &&
    !status.error &&
    !status.error_code
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
  const name = operationLabel(status.operation);
  if (
    status.operation_state === OperationState.Interrupted ||
    status.error_code === RuntimeErrorCode.OperationInterrupted
  )
    return {
      title: i18n.t("operationMessage.interrupted", { name: name }),
      ...(status.error ? { detail: formatErrorMessage(status.error) } : {}),
      tone: "error",
    };
  if (
    status.error_code ||
    status.error ||
    status.operation_state === OperationState.Failed
  ) {
    return {
      title: i18n.t("operationMessage.failed", { name: name }),
      ...(status.error ? { detail: formatErrorMessage(status.error) } : {}),
      tone: "error",
    };
  }
  if (status.operation_state === OperationState.Succeeded)
    return isCurrentOperationSuccessful(status, currentOperationID)
      ? {
          title: i18n.t("operationMessage.completed", { name: name }),
          tone: "success",
        }
      : null;
  if (status.operation_state === OperationState.Queued)
    return {
      title: i18n.t("operationMessage.queued", { name: name }),
      tone: "info",
    };
  return {
    title:
      status.operation &&
      (status.busy || status.operation_state === OperationState.Running)
        ? i18n.t("operationMessage.inProgress", { name: name })
        : i18n.t("operationMessage.serverStatus"),
    ...(status.progress ? { detail: formatErrorMessage(status.progress) } : {}),
    tone: "info",
  };
}
