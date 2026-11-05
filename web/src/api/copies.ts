import { apiFetch, apiJSON, readJSON, API_REQUEST_TIMEOUT_MS } from "./client";
import { isServer, type Server, type ServerConfiguration } from "./servers";
import { isEmber, type Ember } from "./embers";
import { isPolicy, type BackupPolicy } from "./backups";
import { isRecord } from "../utils/validation";
import i18n from "../i18n";

export interface CopyTemplate {
  server: Server;
  ember: Ember;
  policy: BackupPolicy;
}

export async function getCopyTemplate(
  id: string,
  signal: AbortSignal,
): Promise<CopyTemplate> {
  const body = await readJSON(
    await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/copy`, {
      signal,
    }),
  );
  if (
    !isRecord(body) ||
    !isServer(body.server) ||
    !isEmber(body.ember) ||
    !isPolicy(body.policy)
  )
    throw new Error(i18n.t("serverCopy.unexpectedResponse"));
  return { server: body.server, ember: body.ember, policy: body.policy };
}

export async function createServerCopy(
  id: string,
  input: ServerConfiguration,
  files: boolean,
  policy: BackupPolicy | null,
  ownerID?: number,
): Promise<Server> {
  const body = await readJSON(
    await apiJSON(
      `/api/v1/servers/${encodeURIComponent(id)}/copy`,
      {
        ...input,
        copy_files: files,
        policy,
        ...(ownerID ? { owner_user_id: ownerID } : {}),
      },
      { method: "POST", signal: AbortSignal.timeout(API_REQUEST_TIMEOUT_MS) },
    ),
  );
  if (!isServer(body)) throw new Error(i18n.t("serverCopy.unexpectedResponse"));
  return body;
}

export async function quickCopyServer(
  id: string,
  sparkID: string,
): Promise<Server> {
  const body = await readJSON(
    await apiJSON(
      `/api/v1/servers/${encodeURIComponent(id)}/quick-copy`,
      { spark_id: sparkID },
      { method: "POST", signal: AbortSignal.timeout(API_REQUEST_TIMEOUT_MS) },
    ),
  );
  if (!isServer(body)) throw new Error(i18n.t("serverCopy.unexpectedResponse"));
  return body;
}
