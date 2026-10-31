import i18n from "../i18n";
import { apiFetch, readJSON } from "./client";
import type { BackupMode } from "./backups";

export type ServerAction =
  | "install"
  | "update"
  | "reinstall"
  | "start"
  | "stop"
  | "restart"
  | "kill"
  | "backup"
  | "restore";

export interface RuntimeStatus {
  server_id: string;
  state: string;
  ready: boolean;
  busy: boolean;
  progress?: string;
  error?: string;
  operation_id?: string;
  operation?: string;
  operation_state?:
    "queued" | "running" | "succeeded" | "failed" | "interrupted";
  error_code?: string;
  logs?: string;
  disk_quota_enforced: boolean;
  disk_quota_error?: string;
  disk_quota_error_code?: string;
}

function isRuntimeStatus(value: unknown): value is RuntimeStatus {
  return (
    typeof value === "object" &&
    value !== null &&
    "server_id" in value &&
    typeof value.server_id === "string" &&
    "state" in value &&
    typeof value.state === "string" &&
    "ready" in value &&
    typeof value.ready === "boolean" &&
    "busy" in value &&
    typeof value.busy === "boolean" &&
    "disk_quota_enforced" in value &&
    typeof value.disk_quota_enforced === "boolean" &&
    [
      "progress",
      "error",
      "operation_id",
      "operation",
      "operation_state",
      "error_code",
      "logs",
      "disk_quota_error",
      "disk_quota_error_code",
    ].every(
      (key) => !(key in value) || typeof Reflect.get(value, key) === "string",
    )
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

export function serverAction(
  id: string,
  action: ServerAction,
  backup?: string,
  mode?: "keep" | "clean",
  backupMode?: BackupMode,
): Promise<RuntimeStatus> {
  return apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/actions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action, backup, mode, backup_mode: backupMode }),
    signal: AbortSignal.timeout(35000),
  }).then((response) => readStatus(response, id));
}

export function sendConsole(
  id: string,
  command: string,
): Promise<RuntimeStatus> {
  return apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/console`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ command }),
  }).then((response) => readStatus(response, id));
}
