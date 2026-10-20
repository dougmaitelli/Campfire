import i18n from "../i18n";
import { apiFetch, readJSON } from "./client";
import type { ServerPort } from "./servers";

export interface ServerTransfer {
  id: string;
  server_id: string;
  source_spark_id: string;
  target_spark_id: string;
  phase: string;
  active: boolean;
  can_cancel: boolean;
  error: string;
  transferred_bytes: number;
  total_bytes: number;
  created_at: number;
  updated_at: number;
}

function isTransfer(value: unknown): value is ServerTransfer {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    return false;
  const object = value as Record<string, unknown>;
  return (
    [
      "id",
      "server_id",
      "source_spark_id",
      "target_spark_id",
      "phase",
      "error",
    ].every((key) => typeof object[key] === "string") &&
    typeof object.active === "boolean" &&
    typeof object.can_cancel === "boolean" &&
    ["transferred_bytes", "total_bytes", "created_at", "updated_at"].every(
      (key) =>
        typeof object[key] === "number" &&
        Number.isSafeInteger(object[key]) &&
        object[key] >= 0,
    )
  );
}

export async function listTransfers(
  serverID: string,
  signal: AbortSignal,
): Promise<ServerTransfer[]> {
  const result = await readJSON(
    await apiFetch(
      `/api/v1/servers/${encodeURIComponent(serverID)}/transfers`,
      { signal },
    ),
  );
  if (!Array.isArray(result) || !result.every(isTransfer))
    throw new Error(i18n.t("transfers.unexpectedTransferHistoryResponse"));
  return result;
}

export async function beginTransfer(
  serverID: string,
  targetSparkID: string,
  ports: ServerPort[],
): Promise<ServerTransfer> {
  const result = await readJSON(
    await apiFetch(
      `/api/v1/servers/${encodeURIComponent(serverID)}/transfers`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ target_spark_id: targetSparkID, ports }),
        signal: AbortSignal.timeout(35000),
      },
    ),
  );
  if (!isTransfer(result))
    throw new Error(i18n.t("transfers.unexpectedTransferResponse"));
  return result;
}

export async function changeTransfer(
  id: string,
  action: "retry" | "cancel",
): Promise<ServerTransfer> {
  const result = await readJSON(
    await apiFetch(`/api/v1/transfers/${encodeURIComponent(id)}/${action}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
      signal: AbortSignal.timeout(15000),
    }),
  );
  if (!isTransfer(result))
    throw new Error(i18n.t("transfers.unexpectedTransferResponse"));
  return result;
}
