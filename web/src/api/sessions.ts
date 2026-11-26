import i18n from "../i18n";
import { isRecord } from "../utils/validation";
import { apiFetch, readJSON } from "./client";

export interface LoginSession {
  id: string;
  user_agent: string;
  created_at: number;
  last_seen_at: number;
  expires_at: number;
  current: boolean;
}

function isSession(value: unknown): value is LoginSession {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.user_agent === "string" &&
    typeof value.current === "boolean" &&
    ["created_at", "last_seen_at", "expires_at"].every(
      (key) =>
        typeof value[key] === "number" && Number.isSafeInteger(value[key]),
    )
  );
}

export async function listSessions(
  signal?: AbortSignal,
): Promise<LoginSession[]> {
  const body = await readJSON(
    await apiFetch("/api/v1/auth/sessions", { ...(signal ? { signal } : {}) }),
  );
  if (!Array.isArray(body) || !body.every(isSession))
    throw new Error(i18n.t("sessions.unexpectedResponse"));
  return body;
}

export async function revokeSession(id?: string): Promise<void> {
  const path =
    id === undefined
      ? "/api/v1/auth/sessions"
      : `/api/v1/auth/sessions/${encodeURIComponent(id)}`;
  const response = await apiFetch(path, { method: "DELETE" });
  if (response.status !== 204) await readJSON(response);
}
