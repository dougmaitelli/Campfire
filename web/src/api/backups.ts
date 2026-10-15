import { apiFetch, readJSON } from "./client";

export interface Backup {
  name: string;
  size: number;
  modified_at: string;
  local: boolean;
  remote: boolean;
  state: string;
  error?: string;
}

export interface BackupPolicy {
  storage: "local" | "s3";
  local_keep_last: number;
  local_max_age_days: number;
  remote_keep_last: number;
  remote_max_age_days: number;
}

export interface BackupListing {
  entries: Backup[];
  policy: BackupPolicy;
  s3_available: boolean;
}

function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isPolicy(value: unknown): value is BackupPolicy {
  return (
    object(value) &&
    (value.storage === "local" || value.storage === "s3") &&
    [
      "local_keep_last",
      "local_max_age_days",
      "remote_keep_last",
      "remote_max_age_days",
    ].every(
      (key) =>
        typeof value[key] === "number" &&
        Number.isSafeInteger(value[key]) &&
        value[key] >= 0,
    )
  );
}

function isBackup(value: unknown): value is Backup {
  return (
    object(value) &&
    typeof value.name === "string" &&
    /^[A-Za-z0-9_-]{1,128}\.tar\.gz$/.test(value.name) &&
    typeof value.size === "number" &&
    Number.isSafeInteger(value.size) &&
    value.size >= 0 &&
    typeof value.modified_at === "string" &&
    Number.isFinite(Date.parse(value.modified_at)) &&
    typeof value.local === "boolean" &&
    typeof value.remote === "boolean" &&
    typeof value.state === "string" &&
    (!("error" in value) || typeof value.error === "string")
  );
}

export async function listBackups(
  id: string,
  signal: AbortSignal,
): Promise<BackupListing> {
  const body = await readJSON(
    await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/backups`, {
      signal,
    }),
  );
  if (
    !object(body) ||
    body.server_id !== id ||
    !Array.isArray(body.entries) ||
    !body.entries.every(isBackup) ||
    !isPolicy(body.policy) ||
    typeof body.s3_available !== "boolean"
  )
    throw new Error("Unexpected backup response.");
  return {
    entries: body.entries,
    policy: body.policy,
    s3_available: body.s3_available,
  };
}

export async function saveBackupPolicy(
  id: string,
  policy: BackupPolicy,
): Promise<BackupPolicy> {
  const body = await readJSON(
    await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/backup-policy`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(policy),
      signal: AbortSignal.timeout(15000),
    }),
  );
  if (!isPolicy(body)) throw new Error("Unexpected backup policy response.");
  return body;
}

export async function deleteBackup(
  id: string,
  name: string,
  storage: "local" | "s3",
): Promise<void> {
  await readJSON(
    await apiFetch(
      `/api/v1/servers/${encodeURIComponent(id)}/backups/${encodeURIComponent(name)}?storage=${storage}`,
      {
        method: "DELETE",
        signal: AbortSignal.timeout(30000),
      },
    ),
  );
}
