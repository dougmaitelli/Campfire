import { apiFetch, readJSON } from "./client";

export interface Notification {
  id: number;
  target_type: string;
  target_id: string;
  kind: string;
  severity: string;
  title: string;
  body: string;
  created_at: number;
  read_at: number;
}
export interface NotificationStatus {
  apprise_enabled: boolean;
  pending: number;
  failed: number;
  cpu_percent: number;
  memory_percent: number;
  disk_percent: number;
  duration_seconds: number;
  interval_seconds: number;
  health_enabled: boolean;
}
function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
function isNotification(value: unknown): value is Notification {
  return (
    object(value) &&
    ["id", "created_at", "read_at"].every(
      (key) =>
        typeof value[key] === "number" && Number.isSafeInteger(value[key]),
    ) &&
    ["target_type", "target_id", "kind", "severity", "title", "body"].every(
      (key) => typeof value[key] === "string",
    )
  );
}
export async function listNotifications(
  signal: AbortSignal,
  before = 0,
  unread = false,
): Promise<Notification[]> {
  const query = new URLSearchParams({ unread: String(unread) });
  if (before) query.set("before", String(before));
  const body = await readJSON(
    await apiFetch(`/api/v1/notifications?${query}`, { signal }),
  );
  if (!Array.isArray(body) || !body.every(isNotification))
    throw new Error("Unexpected notification response.");
  return body;
}
export async function unreadNotifications(
  signal: AbortSignal,
): Promise<number> {
  const body = await readJSON(
    await apiFetch("/api/v1/notifications/unread", { signal }),
  );
  if (
    !object(body) ||
    typeof body.unread !== "number" ||
    !Number.isSafeInteger(body.unread)
  )
    throw new Error("Unexpected unread count.");
  return body.unread;
}
export async function markNotificationRead(id: number, read: boolean) {
  const response = await apiFetch(`/api/v1/notifications/${id}/read`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ read }),
  });
  if (response.status !== 204) await readJSON(response);
}
export async function markAllNotificationsRead(through: number) {
  const response = await apiFetch("/api/v1/notifications/read-all", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ through_id: through }),
  });
  if (response.status !== 204) await readJSON(response);
}
export async function notificationStatus(
  signal: AbortSignal,
): Promise<NotificationStatus> {
  const body = await readJSON(
    await apiFetch("/api/v1/notifications/status", { signal }),
  );
  if (
    !object(body) ||
    typeof body.apprise_enabled !== "boolean" ||
    typeof body.health_enabled !== "boolean" ||
    ![
      "pending",
      "failed",
      "cpu_percent",
      "memory_percent",
      "disk_percent",
      "duration_seconds",
      "interval_seconds",
    ].every(
      (key) => typeof body[key] === "number" && Number.isFinite(body[key]),
    )
  )
    throw new Error("Unexpected notification status.");
  return body as unknown as NotificationStatus;
}
export async function testNotification() {
  await readJSON(
    await apiFetch("/api/v1/notifications/test", { method: "POST" }),
  );
}
export async function retryNotificationDeliveries() {
  const response = await apiFetch("/api/v1/notifications/retry-deliveries", {
    method: "POST",
  });
  if (response.status !== 204) await readJSON(response);
}
