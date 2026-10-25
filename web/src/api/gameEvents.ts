import i18n from "../i18n";
import { apiFetch, readJSON } from "./client";

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
