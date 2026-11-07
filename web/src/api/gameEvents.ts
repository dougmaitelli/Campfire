import { GameLimits } from "../config/policies";
import { isRecord as object } from "../utils/validation";
import i18n from "../i18n";
import { apiFetch, readJSON } from "./client";

export interface GameAttachment {
  id: string;
  name: string;
  content_type: "image/png" | "image/jpeg" | "video/mp4" | "video/webm";
  size: number;
}

export function gameMediaURL(
  serverID: string,
  eventID: number,
  attachmentID: string,
) {
  return `/api/v1/servers/${encodeURIComponent(serverID)}/events/${eventID}/media/${encodeURIComponent(attachmentID)}`;
}

export interface GameEvent {
  id: number;
  server_id: string;
  spark_id: string;
  event_id: string;
  type: string;
  title: string;
  occurred_at: string;
  received_at: number;
  data: Record<string, string | number | boolean>;
  attachments?: GameAttachment[];
}

export interface GameEventPage {
  items: GameEvent[];
  next_before?: number;
}
export interface GameEventFilterOptions {
  types: string[];
  fields: Record<string, string[]>;
}
export interface GameEventFilter {
  search?: string;
  fields?: Record<string, string>;
  type?: string;
  from?: number;
  to?: number;
  before?: number;
}

function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}
function isAttachment(value: unknown): value is GameAttachment {
  return (
    object(value) &&
    typeof value.id === "string" &&
    /^[A-Za-z0-9_-]{1,64}$/.test(value.id) &&
    typeof value.name === "string" &&
    value.name.length > 0 &&
    value.name.length <= GameLimits.MediaNameLength &&
    typeof value.content_type === "string" &&
    ["image/png", "image/jpeg", "video/mp4", "video/webm"].includes(
      value.content_type,
    ) &&
    positive(value.size) &&
    value.size <= GameLimits.MediaBytes
  );
}
function isEvent(value: unknown, serverID: string): value is GameEvent {
  return (
    object(value) &&
    positive(value.id) &&
    value.server_id === serverID &&
    typeof value.spark_id === "string" &&
    typeof value.event_id === "string" &&
    typeof value.type === "string" &&
    typeof value.title === "string" &&
    typeof value.occurred_at === "string" &&
    Number.isFinite(Date.parse(value.occurred_at)) &&
    positive(value.received_at) &&
    value.received_at <= 253402300799 &&
    (value.attachments === undefined ||
      (Array.isArray(value.attachments) &&
        value.attachments.length <= 4 &&
        value.attachments.every(isAttachment))) &&
    object(value.data) &&
    Object.values(value.data).every(
      (field) =>
        typeof field === "string" ||
        typeof field === "boolean" ||
        (typeof field === "number" && Number.isFinite(field)),
    )
  );
}

export async function readGameEventFilterOptions(
  serverID: string,
  signal: AbortSignal,
): Promise<GameEventFilterOptions> {
  const value = await readJSON(
    await apiFetch(
      `/api/v1/servers/${encodeURIComponent(serverID)}/events/filters`,
      { signal },
    ),
  );
  if (
    !object(value) ||
    !Array.isArray(value.types) ||
    !value.types.every((type) => typeof type === "string") ||
    !object(value.fields) ||
    !Object.values(value.fields).every(
      (values) =>
        Array.isArray(values) &&
        values.every((entry) => typeof entry === "string"),
    )
  )
    throw new Error(i18n.t("gameEvents.unexpectedResponse"));
  return value as unknown as GameEventFilterOptions;
}

export async function readGameEvents(
  serverID: string,
  filter: GameEventFilter,
  signal: AbortSignal,
): Promise<GameEventPage> {
  const query = new URLSearchParams({ limit: "25" });
  for (const [key, value] of Object.entries(filter))
    if (key !== "fields" && value !== undefined && value !== "")
      query.set(key, String(value));
  for (const [field, value] of Object.entries(filter.fields ?? {}))
    query.set(`data.${field}`, value);
  const value = await readJSON(
    await apiFetch(
      `/api/v1/servers/${encodeURIComponent(serverID)}/events?${query}`,
      { signal },
    ),
  );
  if (
    !object(value) ||
    !Array.isArray(value.items) ||
    value.items.length > 25 ||
    !value.items.every((item) => isEvent(item, serverID)) ||
    (value.next_before !== undefined && !positive(value.next_before))
  )
    throw new Error(i18n.t("gameEvents.unexpectedResponse"));
  return value as unknown as GameEventPage;
}
