import { apiJSON, apiFetch, readJSON } from "./client";
import { fileLimit, encodeBytes } from "./files";
import { BackupMode } from "./backups";
import { isEmber, type Ember } from "./embers";
import type { ServerConfiguration } from "./servers";
import { RequestTimeout } from "../config/policies";
import { isRecord, isEnumValue } from "../utils/validation";
import i18n from "../i18n";

export interface BackupImport {
  token: string;
  mode: BackupMode;
  configuration: ServerConfiguration;
  ember: Ember;
  import_ember: boolean;
}

function isConfiguration(value: unknown): value is ServerConfiguration {
  return (
    isRecord(value) &&
    ["name", "description", "spark_id", "ember_id", "image"].every(
      (key) => typeof value[key] === "string",
    ) &&
    ["cpu_limit", "memory_mb", "disk_mb"].every(
      (key) => typeof value[key] === "number" && Number.isFinite(value[key]),
    ) &&
    isRecord(value.variables) &&
    Object.values(value.variables).every((item) => typeof item === "string") &&
    (value.ports === null ||
      (Array.isArray(value.ports) && value.ports.length === 0)) &&
    (value.startup_command === undefined ||
      typeof value.startup_command === "string") &&
    (value.startup_override === undefined ||
      typeof value.startup_override === "string") &&
    (value.game_integration === undefined ||
      typeof value.game_integration === "boolean")
  );
}

export async function cancelBackupImport(token: string): Promise<void> {
  await apiFetch(`/api/v1/backup-imports/${encodeURIComponent(token)}`, {
    method: "DELETE",
  });
}

export async function uploadPortableBackup(
  file: File,
  serverID: string | undefined,
  signal: AbortSignal,
  progress: (bytes: number | null) => void,
): Promise<BackupImport> {
  const begun = await readJSON(
    await apiJSON(
      "/api/v1/backup-imports",
      { server_id: serverID, size: file.size },
      { signal },
    ),
  );
  if (!isRecord(begun) || typeof begun.token !== "string")
    throw new Error(i18n.t("backupImports.invalidResponse"));
  const token = begun.token;
  try {
    for (let offset = 0; offset < file.size; offset += fileLimit) {
      signal.throwIfAborted();
      const bytes = new Uint8Array(
        await file.slice(offset, offset + fileLimit).arrayBuffer(),
      );
      await readJSON(
        await apiJSON(
          `/api/v1/backup-imports/${encodeURIComponent(token)}/chunks`,
          { offset, data: encodeBytes(bytes) },
          {
            signal: AbortSignal.any([
              signal,
              AbortSignal.timeout(RequestTimeout.Default),
            ]),
          },
        ),
      );
      progress(offset + bytes.length);
    }
    progress(null);
    const body = await readJSON(
      await apiJSON(
        `/api/v1/backup-imports/${encodeURIComponent(token)}/inspect`,
        {},
        {
          signal: AbortSignal.any([
            signal,
            AbortSignal.timeout(RequestTimeout.Archive),
          ]),
        },
      ),
    );
    if (
      !isRecord(body) ||
      body.token !== token ||
      !isEnumValue(BackupMode, body.mode) ||
      !isEmber(body.ember) ||
      !isConfiguration(body.configuration) ||
      typeof body.import_ember !== "boolean"
    )
      throw new Error(i18n.t("backupImports.invalidResponse"));
    return {
      token,
      mode: body.mode,
      ember: body.ember,
      configuration: { ...body.configuration, ports: [] },
      import_ember: body.import_ember,
    };
  } catch (cause) {
    await cancelBackupImport(token).catch(() => undefined);
    throw cause;
  }
}

export async function publishBackupImport(
  serverID: string,
  token: string,
  signal?: AbortSignal,
): Promise<string> {
  const body = await readJSON(
    await apiJSON(
      `/api/v1/servers/${encodeURIComponent(serverID)}/backup-imports/${encodeURIComponent(token)}`,
      {},
      { signal: signal ?? AbortSignal.timeout(RequestTimeout.Archive) },
    ),
  );
  if (!isRecord(body) || typeof body.name !== "string")
    throw new Error(i18n.t("backupImports.invalidResponse"));
  return body.name;
}
