import { apiFetch, apiJSON, readJSON } from "./client";
import {
  isDetectedLogAlert,
  type DetectedLogAlert,
} from "../contracts/logAlerts";
import i18n from "../i18n";

export async function listLogAlerts(
  serverID: string,
  signal: AbortSignal,
): Promise<DetectedLogAlert[]> {
  const value = await readJSON(
    await apiFetch(
      `/api/v1/servers/${encodeURIComponent(serverID)}/log-alerts`,
      { signal },
    ),
  );
  if (!Array.isArray(value) || !value.every(isDetectedLogAlert))
    throw new Error(i18n.t("logAlerts.unexpectedResponse"));
  return value;
}

export async function dismissLogAlert(
  serverID: string,
  id: string,
): Promise<void> {
  const response = await apiJSON(
    `/api/v1/servers/${encodeURIComponent(serverID)}/log-alerts/${encodeURIComponent(id)}/dismiss`,
    {},
    { method: "POST" },
  );
  if (!response.ok) await readJSON(response);
}
