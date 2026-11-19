import i18n from "../i18n";
import { isPage as isNumberedPage, pageQuery, type Page } from "./pagination";
import { isRecord, isEnumValue } from "../utils/validation";
import {
  ActivityFilterKind,
  ActivityEntryKind,
  ExecutionState,
  JobStatus,
  NotificationKind,
  NotificationTarget,
} from "../contracts/domains";
import { apiFetch, apiJSON, readJSON } from "./client";
import {
  isGameEvent,
  isGameAttachment,
  type GameEvent,
  type GameEventFilterOptions,
} from "./gameEvents";
import { isExecution, type Execution } from "./activity";
import { isJob, type ServerJob } from "./jobs";
import {
  isDetectedLogAlert,
  type DetectedLogAlert,
} from "../contracts/logAlerts";
import {
  isGameFieldLabel,
  validGameFieldLabels,
  type GameFieldLabels,
} from "./gameFields";

export type ActivityKind = "" | ActivityFilterKind;
export const MAX_ACTIVITY_TIME = 253402300799;
export interface ActivityFilter {
  kind?: ActivityKind;
  search?: string;
  state?: string;
  type?: string;
  from?: number;
  to?: number;
  fields?: Record<string, string>;
  page?: number | undefined;
  page_size?: number | undefined;
}
export interface ActivityEntry {
  key: string;
  kind: ActivityEntryKind;
  source_key?: string;
  source_available: boolean;
  created_at: number;
  title: string;
  state: string;
  delivery_check_pending?: boolean;
  provider?: string;
  destination?: string;
  attachment_name?: string;
  attachment_content_type?: string;
  notification_count: number;
}
export type ActivityPage = Page<ActivityEntry>;
// Activity also carries open-ended provider states, so membership is checked
// without pretending every entry is an execution or a job.
const activeActivityStates: ReadonlySet<string> = new Set<string>([
  ExecutionState.Pending,
  ExecutionState.Running,
  ExecutionState.Dispatching,
  JobStatus.Cancelling,
]);
export function activityInProgress(entry: ActivityEntry): boolean {
  return (
    entry.delivery_check_pending === true ||
    activeActivityStates.has(entry.state)
  );
}
export interface ActivityChoices {
  game: GameEventFilterOptions;
}
export interface ActivityNotification {
  binding_id?: string;
  message_id: number;
  name: string;
  body: string;
  severity?: string;
  data?: Record<string, string | number | boolean>;
  field_labels?: GameFieldLabels;
  attachment?: {
    game_event_id: number;
    media: NonNullable<GameEvent["attachments"]>[number];
  };
  attempts: number;
  next_attempt_at: number;
  delivered_at: number;
  error: string;
  can_retry: boolean;
  external_url?: string;
}
export interface ActivityDetails extends ActivityEntry {
  game_event?: GameEvent;
  alert?: DetectedLogAlert;
  monitoring_alert?: {
    id: string;
    target_type: NotificationTarget.Server;
    target_id: string;
    code: string;
    kind: NotificationKind.Alert | NotificationKind.Recovery;
    severity: string;
    title: string;
    body: string;
    created_at: number;
  };
  operation?: Execution;
  job?: ServerJob;
  notification?: ActivityNotification;
  can_retry_operation: boolean;
}
function strings(value: Record<string, unknown>, names: string[]) {
  return names.every((name) => typeof value[name] === "string");
}
function number(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}
function isEntry(value: unknown): value is ActivityEntry {
  return (
    isRecord(value) &&
    strings(value, ["key", "kind", "title", "state"]) &&
    isEnumValue(ActivityEntryKind, value.kind) &&
    number(value.created_at) &&
    value.created_at <= MAX_ACTIVITY_TIME &&
    number(value.notification_count) &&
    typeof value.source_available === "boolean" &&
    (value.delivery_check_pending === undefined ||
      typeof value.delivery_check_pending === "boolean") &&
    [
      "source_key",
      "provider",
      "destination",
      "attachment_name",
      "attachment_content_type",
    ].every((key) => value[key] === undefined || typeof value[key] === "string")
  );
}
function isPage(value: unknown): value is ActivityPage {
  return isNumberedPage(value, isEntry);
}
function isNotification(value: unknown): value is ActivityNotification {
  if (
    !isRecord(value) ||
    !strings(value, ["name", "body", "error"]) ||
    !["message_id", "attempts", "next_attempt_at", "delivered_at"].every(
      (key) => number(value[key]),
    ) ||
    typeof value.can_retry !== "boolean"
  )
    return false;
  if (
    value.attachment !== undefined &&
    (!isRecord(value.attachment) ||
      !number(value.attachment.game_event_id) ||
      value.attachment.game_event_id < 1 ||
      !isGameAttachment(value.attachment.media))
  )
    return false;
  if (
    value.data !== undefined &&
    (!isRecord(value.data) ||
      !Object.values(value.data).every(
        (item) =>
          typeof item === "string" ||
          typeof item === "boolean" ||
          (typeof item === "number" && Number.isFinite(item)),
      ))
  )
    return false;
  if (
    !validGameFieldLabels(
      value.field_labels,
      isRecord(value.data) ? value.data : {},
    )
  )
    return false;
  if (
    value.external_url !== undefined &&
    (typeof value.external_url !== "string" ||
      !/^https:\/\/discord\.com\/channels\/\d+\/\d+\/\d+$/.test(
        value.external_url,
      ))
  )
    return false;
  return ["binding_id", "severity"].every(
    (key) => value[key] === undefined || typeof value[key] === "string",
  );
}
export function activityQuery(filter: ActivityFilter): URLSearchParams {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(filter))
    if (key !== "fields" && value !== undefined && value !== "")
      query.set(key, String(value));
  for (const [key, value] of Object.entries(filter.fields ?? {}))
    query.set(`data.${key}`, value);
  return query;
}
export function serverActivityURL(
  serverID: string,
  filter: ActivityFilter = {},
  entry?: string,
): string {
  const query = activityQuery(filter);
  if (entry) query.set("entry", entry);
  return `#servers/${encodeURIComponent(serverID)}/activity${query.size ? `?${query}` : ""}`;
}
export function activityFilterFromQuery(
  query: URLSearchParams,
): ActivityFilter {
  const kind = query.get("kind") ?? "";
  const filter: ActivityFilter = {
    kind: isEnumValue(ActivityFilterKind, kind) ? kind : "",
  };
  for (const key of ["search", "type"] as const) {
    const value = query.get(key);
    if (value) filter[key] = value;
  }
  const size = Number(query.get("page_size"));
  if (
    query.has("page_size") &&
    Number.isSafeInteger(size) &&
    size >= 1 &&
    size <= 100
  )
    filter.page_size = size;
  const page = Number(query.get("page") ?? 1);
  if (
    query.has("page") &&
    Number.isSafeInteger(page) &&
    page > 0 &&
    page <= 21474836
  )
    filter.page = page;
  if (
    filter.kind === ActivityFilterKind.Operations ||
    filter.kind === ActivityFilterKind.Alert
  ) {
    const state = query.get("state");
    if (state) filter.state = state;
  }
  for (const key of ["from", "to"] as const) {
    const value = query.get(key);
    if (value && number(Number(value)) && Number(value) <= MAX_ACTIVITY_TIME)
      filter[key] = Number(value);
  }
  const fields: Record<string, string> = {};
  for (const [key, value] of query)
    if (key.startsWith("data.")) fields[key.slice(5)] = value;
  if (Object.keys(fields).length) filter.fields = fields;
  return filter;
}
export async function readActivity(
  serverID: string,
  filter: ActivityFilter,
  signal: AbortSignal,
): Promise<ActivityPage> {
  return readActivityPage(
    `/api/v1/servers/${encodeURIComponent(serverID)}/activity?${activityQuery(filter)}`,
    signal,
  );
}

export async function readRelatedNotifications(
  serverID: string,
  source: string,
  page: number,
  signal: AbortSignal,
  pageSize?: number,
): Promise<ActivityPage> {
  const query = `?${pageQuery(page, pageSize)}`;
  return readActivityPage(
    `/api/v1/servers/${encodeURIComponent(serverID)}/activity/${encodeURIComponent(source)}/notifications${query}`,
    signal,
  );
}

async function readActivityPage(
  url: string,
  signal: AbortSignal,
): Promise<ActivityPage> {
  const value = await readJSON(await apiFetch(url, { signal }));
  if (!isPage(value))
    throw new Error(i18n.t("activity.unexpectedActivityResponse"));
  return value;
}
function isDetails(value: unknown, serverID: string): value is ActivityDetails {
  return (
    isEntry(value) &&
    isRecord(value) &&
    typeof value.can_retry_operation === "boolean" &&
    (value.game_event === undefined ||
      isGameEvent(value.game_event, serverID)) &&
    (value.alert === undefined || isDetectedLogAlert(value.alert)) &&
    (value.monitoring_alert === undefined ||
      (isRecord(value.monitoring_alert) &&
        strings(value.monitoring_alert, [
          "target_id",
          "code",
          "severity",
          "title",
          "body",
        ]) &&
        typeof value.monitoring_alert.id === "string" &&
        value.monitoring_alert.target_type === NotificationTarget.Server &&
        value.monitoring_alert.target_id === serverID &&
        (value.monitoring_alert.kind === NotificationKind.Alert ||
          value.monitoring_alert.kind === NotificationKind.Recovery) &&
        number(value.monitoring_alert.created_at) &&
        value.key === `monitoring_alert:${value.monitoring_alert.id}`)) &&
    (value.operation === undefined ||
      (isExecution(value.operation) &&
        value.operation.server_id === serverID &&
        value.key === `operation:${value.operation.id}`)) &&
    (value.job === undefined ||
      (isJob(value.job) &&
        value.key === `job:${value.job.id}` &&
        [value.job.source_server_id, value.job.target_server_id].includes(
          serverID,
        ))) &&
    (value.notification === undefined || isNotification(value.notification))
  );
}
export async function readActivityDetails(
  serverID: string,
  key: string,
  signal: AbortSignal,
): Promise<ActivityDetails> {
  const value = await readJSON(
    await apiFetch(
      `/api/v1/servers/${encodeURIComponent(serverID)}/activity/${encodeURIComponent(key)}`,
      { signal },
    ),
  );
  if (!isDetails(value, serverID))
    throw new Error(i18n.t("activity.unexpectedActivityResponse"));
  return value;
}
export async function readActivityChoices(
  serverID: string,
  signal: AbortSignal,
): Promise<ActivityChoices> {
  const value = await readJSON(
    await apiFetch(
      `/api/v1/servers/${encodeURIComponent(serverID)}/activity/filters?kind=game`,
      { signal },
    ),
  );
  if (
    !isRecord(value) ||
    !isRecord(value.game) ||
    !Array.isArray(value.game.types) ||
    !value.game.types.every((item) => typeof item === "string") ||
    !isRecord(value.game.fields) ||
    !Object.values(value.game.fields).every(
      (items) =>
        Array.isArray(items) && items.every((item) => typeof item === "string"),
    ) ||
    (value.game.field_labels !== undefined &&
      (!isRecord(value.game.field_labels) ||
        !Object.values(value.game.field_labels).every(
          (labels) =>
            isRecord(labels) && Object.values(labels).every(isGameFieldLabel),
        )))
  )
    throw new Error(i18n.t("activity.unexpectedActivityResponse"));
  return value as unknown as ActivityChoices;
}
export async function retryActivityNotification(
  serverID: string,
  key: string,
): Promise<void> {
  const response = await apiJSON(
    `/api/v1/servers/${encodeURIComponent(serverID)}/activity/${encodeURIComponent(key)}/retry`,
    {},
    { method: "POST" },
  );
  if (!response.ok) await readJSON(response);
}
