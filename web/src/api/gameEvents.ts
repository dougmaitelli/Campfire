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
export interface GameEventFilter {
  type?: string;
  from?: number;
  to?: number;
  before?: number;
}

function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
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
    value.name.length <= 128 &&
    typeof value.content_type === "string" &&
    ["image/png", "image/jpeg", "video/mp4", "video/webm"].includes(
      value.content_type,
    ) &&
    positive(value.size) &&
    value.size <= 25 * 1024 * 1024
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

export async function readGameEvents(
  serverID: string,
  filter: GameEventFilter,
  signal: AbortSignal,
): Promise<GameEventPage> {
  const query = new URLSearchParams({ limit: "25" });
  for (const [key, value] of Object.entries(filter))
    if (value !== undefined && value !== "") query.set(key, String(value));
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
