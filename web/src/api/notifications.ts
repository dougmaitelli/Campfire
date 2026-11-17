import {
  NotificationTarget,
  NotificationKind,
  NotificationSeverity,
} from "../contracts/domains";
import { isRecord as object, isEnumValue } from "../utils/validation";
import i18n from "../i18n";
import { apiFetch, readJSON, apiJSON } from "./client";
import { readPage, pageQuery, type Page } from "./pagination";

export const NOTIFICATIONS_REFRESH_INTERVAL_MS = 5000;

export interface Notification {
  id: number;
  target_type: NotificationTarget;
  target_id: string;
  kind: NotificationKind;
  severity: NotificationSeverity;
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

function isNotification(value: unknown): value is Notification {
  return (
    object(value) &&
    ["id", "created_at", "read_at"].every(
      (key) =>
        typeof value[key] === "number" && Number.isSafeInteger(value[key]),
    ) &&
    isEnumValue(NotificationTarget, value.target_type) &&
    isEnumValue(NotificationKind, value.kind) &&
    isEnumValue(NotificationSeverity, value.severity) &&
    ["target_type", "target_id", "kind", "severity", "title", "body"].every(
      (key) => typeof value[key] === "string",
    )
  );
}
export async function listNotifications({
  signal,
  page = 1,
  unread = false,
  pageSize,
}: {
  signal: AbortSignal;
  page?: number;
  unread?: boolean;
  pageSize?: number;
}): Promise<Page<Notification>> {
  const query = pageQuery(page, pageSize);
  query.set("unread", String(unread));
  return readPage(`/api/v1/notifications?${query}`, signal, isNotification);
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
    throw new Error(i18n.t("notifications.unexpectedUnreadCount"));
  return body.unread;
}
export async function markNotificationRead(id: number, read: boolean) {
  const response = await apiJSON(
    `/api/v1/notifications/${id}/read`,
    { read },
    { method: "PUT" },
  );
  if (response.status !== 204) await readJSON(response);
}
export async function markAllNotificationsRead(through: number) {
  const response = await apiJSON(
    "/api/v1/notifications/read-all",
    { through_id: through },
    { method: "POST" },
  );
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
    throw new Error(i18n.t("notifications.unexpectedNotificationStatus"));
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
