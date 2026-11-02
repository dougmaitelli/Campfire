import { isEnumValue } from "../utils/validation";
import {
  RuntimeState,
  RuntimeErrorCode,
  ServerAction,
  OperationState,
  type ReinstallMode,
  type LifecycleAction,
} from "../contracts/domains";
import i18n from "../i18n";
import { apiFetch, readJSON, API_REQUEST_TIMEOUT_MS, apiJSON } from "./client";
import type { BackupMode } from "./backups";

export { ServerAction };

export interface RuntimeStatus {
  server_id: string;
  state: RuntimeState;
  ready: boolean;
  busy: boolean;
  progress?: string;
  error?: string;
  operation_id?: string;
  operation?: ServerAction;
  operation_state?: OperationState;
  error_code?: RuntimeErrorCode;
  logs?: string;
  disk_quota_enforced: boolean;
  disk_quota_error?: string;
  disk_quota_error_code?: RuntimeErrorCode;
}

function isRuntimeStatus(value: unknown): value is RuntimeStatus {
  return (
    typeof value === "object" &&
    value !== null &&
    "server_id" in value &&
    typeof value.server_id === "string" &&
    "state" in value &&
    isEnumValue(RuntimeState, value.state) &&
    "ready" in value &&
    typeof value.ready === "boolean" &&
    "busy" in value &&
    typeof value.busy === "boolean" &&
    "disk_quota_enforced" in value &&
    typeof value.disk_quota_enforced === "boolean" &&
    ["progress", "error", "operation_id", "logs", "disk_quota_error"].every(
      (key) => !(key in value) || typeof Reflect.get(value, key) === "string",
    ) &&
    (!("operation" in value) || isEnumValue(ServerAction, value.operation)) &&
    (!("error_code" in value) ||
      isEnumValue(RuntimeErrorCode, value.error_code)) &&
    (!("disk_quota_error_code" in value) ||
      isEnumValue(RuntimeErrorCode, value.disk_quota_error_code)) &&
    (!("operation_state" in value) ||
      isEnumValue(OperationState, value.operation_state))
  );
}

async function readStatus(
  response: Response,
  id: string,
): Promise<RuntimeStatus> {
  const result = await readJSON(response);
  if (!isRuntimeStatus(result) || result.server_id !== id)
    throw new Error(i18n.t("runtime.unexpectedRuntimeResponse"));
  return result;
}

export function serverStatus(
  id: string,
  signal: AbortSignal,
): Promise<RuntimeStatus> {
  return apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/status`, {
    signal,
  }).then((response) => readStatus(response, id));
}

export async function listServerStatuses(
  signal: AbortSignal,
): Promise<RuntimeStatus[]> {
  const result = await readJSON(
    await apiFetch("/api/v1/servers/status", { signal }),
  );
  if (!Array.isArray(result) || !result.every(isRuntimeStatus))
    throw new Error(i18n.t("runtime.unexpectedRuntimeResponse"));
  return result;
}

export function serverAction(
  id: string,
  action: LifecycleAction,
  backup?: string,
  mode?: ReinstallMode,
  backupMode?: BackupMode,
): Promise<RuntimeStatus> {
  return apiJSON(
    `/api/v1/servers/${encodeURIComponent(id)}/actions`,
    { action, backup, mode, backup_mode: backupMode },
    { method: "POST", signal: AbortSignal.timeout(API_REQUEST_TIMEOUT_MS) },
  ).then((response) => readStatus(response, id));
}

export function sendConsole(
  id: string,
  command: string,
): Promise<RuntimeStatus> {
  return apiJSON(
    `/api/v1/servers/${encodeURIComponent(id)}/console`,
    { command },
    { method: "POST" },
  ).then((response) => readStatus(response, id));
}
