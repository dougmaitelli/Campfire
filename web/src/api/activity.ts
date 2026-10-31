import i18n from "../i18n";
import { apiFetch, readJSON } from "./client";
import { isUser, type User } from "./access";
import type { BackupMode } from "./backups";

export interface Schedule {
  backup_mode?: BackupMode;
  id: string;
  server_id: string;
  name: string;
  action: string;
  cron_expression: string;
  time_zone: string;
  only_when_running: boolean;
  next_run_at: number;
  enabled: boolean;
  run_as_user_id: number;
  username: string;
  created_at: number;
}
export interface ScheduleInput {
  backup_mode?: BackupMode;
  next_run_at?: number;
  server_id: string;
  name: string;
  action: string;
  command: string;
  cron_expression: string;
  time_zone: string;
  only_when_running: boolean;
  enabled: boolean;
}
export interface Execution {
  backup_mode?: BackupMode;
  id: string;
  schedule_id: string;
  retry_of: string;
  recurring: boolean;
  server_id: string;
  server_name: string;
  user_id: number;
  username: string;
  action: string;
  state: string;
  message: string;
  created_at: number;
  started_at: number;
  finished_at: number;
}
export interface AuditEvent {
  id: string;
  api_token_id?: number;
  user_id: number | null;
  user?: User;
  username: string;
  action: string;
  target: string;
  outcome: string;
  status: number;
  created_at: number;
}

export function activityCursor(
  item: { created_at: number; id: string } | undefined,
): string {
  return item ? `${item.created_at}:${item.id}` : "";
}
function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function strings(value: Record<string, unknown>, keys: string[]) {
  return keys.every((key) => typeof value[key] === "string");
}
function numbers(value: Record<string, unknown>, keys: string[]) {
  return keys.every(
    (key) => typeof value[key] === "number" && Number.isSafeInteger(value[key]),
  );
}
function isSchedule(value: unknown): value is Schedule {
  return (
    object(value) &&
    (!("backup_mode" in value) ||
      value.backup_mode === "full" ||
      value.backup_mode === "savedata") &&
    strings(value, [
      "id",
      "server_id",
      "name",
      "action",
      "username",
      "cron_expression",
      "time_zone",
    ]) &&
    numbers(value, ["next_run_at", "run_as_user_id", "created_at"]) &&
    typeof value.enabled === "boolean" &&
    typeof value.only_when_running === "boolean"
  );
}
function isExecution(value: unknown): value is Execution {
  return (
    object(value) &&
    strings(value, [
      "id",
      "schedule_id",
      "retry_of",
      "server_id",
      "server_name",
      "username",
      "action",
      "state",
      "message",
    ]) &&
    numbers(value, ["user_id", "created_at", "started_at", "finished_at"]) &&
    typeof value.recurring === "boolean"
  );
}
function isAudit(value: unknown): value is AuditEvent {
  return (
    object(value) &&
    (value.api_token_id === undefined ||
      (typeof value.api_token_id === "number" &&
        Number.isSafeInteger(value.api_token_id))) &&
    strings(value, ["id", "username", "action", "target", "outcome"]) &&
    (value.user_id === null ||
      (typeof value.user_id === "number" &&
        Number.isSafeInteger(value.user_id))) &&
    (value.user === undefined || isUser(value.user)) &&
    numbers(value, ["status", "created_at"])
  );
}
async function collection<T>(
  url: string,
  signal: AbortSignal,
  validate: (value: unknown) => value is T,
): Promise<T[]> {
  const body = await readJSON(await apiFetch(url, { signal }));
  if (!Array.isArray(body) || !body.every(validate))
    throw new Error(i18n.t("activity.unexpectedActivityResponse"));
  return body;
}
export function listSchedules(signal: AbortSignal) {
  return collection("/api/v1/schedules", signal, isSchedule);
}
export function listExecutions(
  signal: AbortSignal,
  before = "",
  serverID = "",
  scheduleID = "",
) {
  const query = new URLSearchParams({ before, server_id: serverID });
  if (scheduleID) query.set("schedule_id", scheduleID);
  return collection(
    `/api/v1/schedules/executions?${query}`,
    signal,
    isExecution,
  );
}
export function listAudit(
  signal: AbortSignal,
  before = "",
  action = "",
  target = "",
) {
  const query = new URLSearchParams({ before, action, target });
  return collection(`/api/v1/audit?${query}`, signal, isAudit);
}
async function send(url: string, method: string, input?: unknown) {
  const response = await apiFetch(url, {
    method,
    headers: { "Content-Type": "application/json" },
    ...(input === undefined ? {} : { body: JSON.stringify(input) }),
  });
  if (response.status === 204) return null;
  return readJSON(response);
}
export async function previewSchedule(
  expression: string,
  timeZone: string,
  signal: AbortSignal,
): Promise<number[]> {
  const body = await readJSON(
    await apiFetch("/api/v1/schedules/preview", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        cron_expression: expression,
        time_zone: timeZone,
      }),
      signal,
    }),
  );
  if (
    !object(body) ||
    !Array.isArray(body.next_runs) ||
    body.next_runs.length !== 3 ||
    !body.next_runs.every(
      (value: unknown) =>
        typeof value === "number" && Number.isSafeInteger(value) && value > 0,
    )
  )
    throw new Error(i18n.t("activity.unexpectedSchedulePreviewResponse"));
  return body.next_runs as number[];
}

export async function saveSchedule(
  input: ScheduleInput,
  id?: string,
): Promise<Schedule> {
  const body = await send(
    `/api/v1/schedules${id ? `/${encodeURIComponent(id)}` : ""}`,
    id ? "PUT" : "POST",
    input,
  );
  if (!isSchedule(body))
    throw new Error(i18n.t("activity.unexpectedScheduleResponse"));
  return body;
}
export async function deleteSchedule(id: string) {
  await send(`/api/v1/schedules/${encodeURIComponent(id)}`, "DELETE");
}

export async function setScheduleEnabled(
  id: string,
  enabled: boolean,
): Promise<Schedule> {
  const body = await send(
    `/api/v1/schedules/${encodeURIComponent(id)}`,
    "PATCH",
    { enabled },
  );
  if (!isSchedule(body))
    throw new Error(i18n.t("activity.unexpectedScheduleResponse"));
  return body;
}
export async function runSchedule(id: string) {
  await send(`/api/v1/schedules/${encodeURIComponent(id)}/run`, "POST");
}
export async function retryExecution(id: string) {
  await send(
    `/api/v1/schedules/executions/${encodeURIComponent(id)}/retry`,
    "POST",
  );
}
