import { BackupMode, BackupStorage, BackupState } from "../contracts/domains";
import { isRecord as object, isEnumValue } from "../utils/validation";
import i18n from "../i18n";
import { apiFetch, readJSON, API_READ_TIMEOUT_MS, apiJSON } from "./client";

export { BackupMode };

export interface Backup {
  display_name: string;
  pinned: boolean;
  portable?: boolean;
  mode?: BackupMode;
  name: string;
  size: number;
  modified_at: string;
  local: boolean;
  remote: boolean;
  state: BackupState;
  error?: string;
}

export interface BackupRetention {
  local_keep_last: number;
  local_max_age_days: number;
  remote_keep_last: number;
  remote_max_age_days: number;
}

export interface BackupPolicy extends BackupRetention {
  storage: BackupStorage;
  savedata?: BackupRetention;
}

export interface CatalogBackup extends Backup {
  can_download: boolean;
  can_edit: boolean;
  id: string;
  server_id: string;
  server_name: string;
}

export async function listAllBackups(
  signal: AbortSignal,
): Promise<CatalogBackup[]> {
  const body = await readJSON(await apiFetch("/api/v1/backups", { signal }));
  if (!Array.isArray(body))
    throw new Error(i18n.t("backups.unexpectedBackupCatalogResponse"));
  return body.map((value: unknown) => {
    if (
      !isBackup(value) ||
      !object(value) ||
      typeof value.server_id !== "string" ||
      typeof value.server_name !== "string" ||
      typeof value.can_download !== "boolean" ||
      typeof value.can_edit !== "boolean"
    )
      throw new Error(i18n.t("backups.unexpectedBackupCatalogEntry"));
    return {
      ...value,
      server_id: value.server_id,
      server_name: value.server_name,
      can_download: value.can_download,
      can_edit: value.can_edit,
      id: `${value.server_id}/${value.name}`,
    };
  });
}

export async function getBackupPolicy(
  id: string,
  signal: AbortSignal,
): Promise<{ policy: BackupPolicy; s3_available: boolean }> {
  const body = await readJSON(
    await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/backup-policy`, {
      signal,
    }),
  );
  if (
    !object(body) ||
    !isPolicy(body.policy) ||
    typeof body.s3_available !== "boolean"
  )
    throw new Error(i18n.t("backups.unexpectedBackupPolicyResponse"));
  return { policy: body.policy, s3_available: body.s3_available };
}

export interface BackupListing {
  entries: Backup[];
  policy: BackupPolicy;
  s3_available: boolean;
}

function isRetention(value: unknown): value is BackupRetention {
  return (
    object(value) &&
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

export function isPolicy(value: unknown): value is BackupPolicy {
  return (
    object(value) &&
    isRetention(value) &&
    isEnumValue(BackupStorage, value.storage) &&
    (!("savedata" in value) || isRetention(value.savedata))
  );
}

function isBackup(value: unknown): value is Backup {
  return (
    object(value) &&
    typeof value.display_name === "string" &&
    typeof value.pinned === "boolean" &&
    (!("mode" in value) || isEnumValue(BackupMode, value.mode)) &&
    typeof value.name === "string" &&
    /^(?:(?:full|savedata)-)?[A-Za-z0-9_-]{1,128}\.tar\.gz$/.test(value.name) &&
    typeof value.size === "number" &&
    Number.isSafeInteger(value.size) &&
    value.size >= 0 &&
    typeof value.modified_at === "string" &&
    Number.isFinite(Date.parse(value.modified_at)) &&
    typeof value.local === "boolean" &&
    typeof value.remote === "boolean" &&
    isEnumValue(BackupState, value.state) &&
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
    throw new Error(i18n.t("backups.unexpectedBackupResponse"));
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
    await apiJSON(
      `/api/v1/servers/${encodeURIComponent(id)}/backup-policy`,
      policy,
      { method: "PUT", signal: AbortSignal.timeout(API_READ_TIMEOUT_MS) },
    ),
  );
  if (!isPolicy(body))
    throw new Error(i18n.t("backups.unexpectedBackupPolicyResponse"));
  return body;
}

export async function deleteBackup(
  id: string,
  name: string,
  storage: BackupStorage,
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

export async function updateBackup(
  id: string,
  name: string,
  changes: { display_name?: string; pinned?: boolean },
): Promise<Backup> {
  const body = await readJSON(
    await apiJSON(
      `/api/v1/servers/${encodeURIComponent(id)}/backups/${encodeURIComponent(name)}`,
      changes,
      { method: "PATCH", signal: AbortSignal.timeout(API_READ_TIMEOUT_MS) },
    ),
  );
  if (!isBackup(body))
    throw new Error(i18n.t("backups.unexpectedBackupResponse"));
  return body;
}
