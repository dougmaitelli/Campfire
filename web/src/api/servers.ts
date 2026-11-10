import { type PermissionID, isPermissionID } from "./permissions";
import { isEnumValue, isRecord } from "../utils/validation";
import {
  NetworkProtocol,
  RuntimeState,
  VariableType,
} from "../contracts/domains";
import i18n from "../i18n";
import { apiFetch, readJSON, API_REQUEST_TIMEOUT_MS, apiJSON } from "./client";

export interface ServerPort {
  bind_ip: string;
  host_port: number;
  container_port: number;
  protocol: NetworkProtocol;
}

export interface ServerConfiguration {
  game_integration?: boolean;
  name: string;
  description: string;
  spark_id: string;
  ember_id: string;
  image: string;
  startup_command?: string;
  startup_override?: string | undefined;
  variables: Record<string, string>;
  cpu_limit: number;
  memory_mb: number;
  disk_mb: number;
  ports: ServerPort[];
}

export interface Server extends ServerConfiguration {
  savedata_available?: boolean;
  ember_hash: string;
  ember_update_available: boolean;
  ember_review_required?: boolean;
  deployment_sync_required?: boolean;
  id: string;
  owner_user_id: number;
  permissions: PermissionID[];
  can_manage_access?: boolean;
  spark_name: string;
  ember_name: string;
  ember_art_url?: string;
  game_address: string;
  port_descriptions?: Record<string, string>;
  variable_types?: Record<string, VariableType>;
  status: RuntimeState;
  created_at: string;
}

export function isServer(value: unknown): value is Server {
  return (
    isServerConfiguration(value) &&
    (!("savedata_available" in value) ||
      typeof value.savedata_available === "boolean") &&
    "id" in value &&
    typeof value.id === "string" &&
    (!("can_manage_access" in value) ||
      typeof value.can_manage_access === "boolean") &&
    "ember_hash" in value &&
    typeof value.ember_hash === "string" &&
    "ember_update_available" in value &&
    typeof value.ember_update_available === "boolean" &&
    (!("ember_review_required" in value) ||
      typeof value.ember_review_required === "boolean") &&
    (!("deployment_sync_required" in value) ||
      typeof value.deployment_sync_required === "boolean") &&
    "owner_user_id" in value &&
    typeof value.owner_user_id === "number" &&
    Number.isSafeInteger(value.owner_user_id) &&
    "permissions" in value &&
    Array.isArray(value.permissions) &&
    value.permissions.every(isPermissionID) &&
    "game_address" in value &&
    typeof value.game_address === "string" &&
    (!("variable_types" in value) ||
      (typeof value.variable_types === "object" &&
        value.variable_types !== null &&
        !Array.isArray(value.variable_types) &&
        Object.values(value.variable_types).every((type: unknown) =>
          isEnumValue(VariableType, type),
        ))) &&
    (!("port_descriptions" in value) ||
      (typeof value.port_descriptions === "object" &&
        value.port_descriptions !== null &&
        !Array.isArray(value.port_descriptions) &&
        Object.values(value.port_descriptions).every(
          (description: unknown) => typeof description === "string",
        ))) &&
    "spark_name" in value &&
    typeof value.spark_name === "string" &&
    "ember_name" in value &&
    typeof value.ember_name === "string" &&
    (!("ember_art_url" in value) || typeof value.ember_art_url === "string") &&
    "status" in value &&
    isEnumValue(RuntimeState, value.status) &&
    "created_at" in value &&
    typeof value.created_at === "string" &&
    Number.isFinite(Date.parse(value.created_at))
  );
}

export function serverNeedsEmberReview(
  server: Server | null | undefined,
): boolean {
  return (
    server?.ember_review_required ?? server?.ember_update_available ?? false
  );
}

export function isServerConfiguration(
  value: unknown,
): value is ServerConfiguration {
  return (
    isRecord(value) &&
    ["name", "description", "spark_id", "ember_id", "image"].every(
      (key) => typeof value[key] === "string",
    ) &&
    (value.startup_command === undefined ||
      typeof value.startup_command === "string") &&
    (value.startup_override === undefined ||
      typeof value.startup_override === "string") &&
    (value.game_integration === undefined ||
      typeof value.game_integration === "boolean") &&
    typeof value.cpu_limit === "number" &&
    Number.isFinite(value.cpu_limit) &&
    Number.isSafeInteger(value.memory_mb) &&
    Number.isSafeInteger(value.disk_mb) &&
    Array.isArray(value.ports) &&
    value.ports.every(isServerPort) &&
    isRecord(value.variables) &&
    Object.values(value.variables).every((entry) => typeof entry === "string")
  );
}

function isServerPort(value: unknown): value is ServerPort {
  return (
    typeof value === "object" &&
    value !== null &&
    "bind_ip" in value &&
    typeof value.bind_ip === "string" &&
    "host_port" in value &&
    typeof value.host_port === "number" &&
    Number.isInteger(value.host_port) &&
    "container_port" in value &&
    typeof value.container_port === "number" &&
    Number.isInteger(value.container_port) &&
    "protocol" in value &&
    isEnumValue(NetworkProtocol, value.protocol)
  );
}

export async function listServers(signal: AbortSignal): Promise<Server[]> {
  const body = await readJSON(await apiFetch("/api/v1/servers", { signal }));
  if (!Array.isArray(body) || !body.every(isServer))
    throw new Error(i18n.t("servers.unexpectedServerListResponse"));
  return body;
}

export async function saveServer(
  input: ServerConfiguration,
  id?: string,
  backupToken?: string,
): Promise<Server> {
  const body = await readJSON(
    await apiJSON(
      id ? `/api/v1/servers/${encodeURIComponent(id)}` : "/api/v1/servers",
      backupToken && !id ? { ...input, backup_token: backupToken } : input,
      { method: id ? "PUT" : "POST" },
    ),
  );
  if (!isServer(body))
    throw new Error(i18n.t("servers.unexpectedServerResponse"));
  return body;
}

export async function deleteServer(id: string): Promise<void> {
  const response = await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  if (!response.ok) await readJSON(response);
}

export async function applyServerEmber(
  id: string,
  expectedEmberID: string,
  input: ServerConfiguration,
  expectedEmberRevision?: string,
): Promise<Server> {
  const body = await readJSON(
    await apiJSON(
      `/api/v1/servers/${encodeURIComponent(id)}/ember`,
      {
        ...input,
        expected_ember_id: expectedEmberID,
        expected_ember_revision: expectedEmberRevision,
      },
      { method: "POST", signal: AbortSignal.timeout(API_REQUEST_TIMEOUT_MS) },
    ),
  );
  if (!isServer(body))
    throw new Error(i18n.t("servers.unexpectedServerResponse"));
  return body;
}

export async function syncServerConfiguration(
  id: string,
  signal: AbortSignal,
): Promise<void> {
  const body = await readJSON(
    await apiJSON(
      `/api/v1/servers/${encodeURIComponent(id)}/configuration`,
      {},
      { method: "POST", signal },
    ),
  );
  if (
    !isRecord(body) ||
    body.server_id !== id ||
    typeof body.revision !== "string"
  )
    throw new Error(i18n.t("servers.unexpectedServerResponse"));
}
