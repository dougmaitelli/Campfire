import i18n from "../i18n";
import { apiFetch, readJSON } from "./client";

export interface ServerPort {
  bind_ip: string;
  host_port: number;
  container_port: number;
  protocol: "tcp" | "udp";
}

export interface ServerConfiguration {
  name: string;
  description: string;
  spark_id: string;
  ember_id: string;
  image: string;
  startup_command?: string;
  variables: Record<string, string>;
  cpu_limit: number;
  memory_mb: number;
  disk_mb: number;
  ports: ServerPort[];
}

export interface Server extends ServerConfiguration {
  ember_hash: string;
  ember_update_available: boolean;
  id: string;
  owner_user_id: number;
  permissions: string[];
  spark_name: string;
  ember_name: string;
  ember_art_url?: string;
  game_address: string;
  port_descriptions?: Record<string, string>;
  status: string;
  created_at: string;
}

export function isServer(value: unknown): value is Server {
  return (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof value.id === "string" &&
    "ember_hash" in value &&
    typeof value.ember_hash === "string" &&
    "ember_update_available" in value &&
    typeof value.ember_update_available === "boolean" &&
    "owner_user_id" in value &&
    typeof value.owner_user_id === "number" &&
    Number.isSafeInteger(value.owner_user_id) &&
    "permissions" in value &&
    Array.isArray(value.permissions) &&
    value.permissions.every((entry: unknown) => typeof entry === "string") &&
    "name" in value &&
    typeof value.name === "string" &&
    "description" in value &&
    typeof value.description === "string" &&
    "game_address" in value &&
    typeof value.game_address === "string" &&
    (!("port_descriptions" in value) ||
      (typeof value.port_descriptions === "object" &&
        value.port_descriptions !== null &&
        !Array.isArray(value.port_descriptions) &&
        Object.values(value.port_descriptions).every(
          (description: unknown) => typeof description === "string",
        ))) &&
    "spark_id" in value &&
    typeof value.spark_id === "string" &&
    "spark_name" in value &&
    typeof value.spark_name === "string" &&
    "ember_id" in value &&
    typeof value.ember_id === "string" &&
    "ember_name" in value &&
    typeof value.ember_name === "string" &&
    (!("ember_art_url" in value) || typeof value.ember_art_url === "string") &&
    "image" in value &&
    typeof value.image === "string" &&
    (!("startup_command" in value) ||
      typeof value.startup_command === "string") &&
    "cpu_limit" in value &&
    typeof value.cpu_limit === "number" &&
    Number.isFinite(value.cpu_limit) &&
    "memory_mb" in value &&
    typeof value.memory_mb === "number" &&
    Number.isSafeInteger(value.memory_mb) &&
    "disk_mb" in value &&
    typeof value.disk_mb === "number" &&
    Number.isSafeInteger(value.disk_mb) &&
    "ports" in value &&
    Array.isArray(value.ports) &&
    value.ports.every(isServerPort) &&
    "variables" in value &&
    typeof value.variables === "object" &&
    value.variables !== null &&
    !Array.isArray(value.variables) &&
    Object.values(value.variables).every(
      (entry: unknown) => typeof entry === "string",
    ) &&
    "status" in value &&
    typeof value.status === "string" &&
    "created_at" in value &&
    typeof value.created_at === "string" &&
    Number.isFinite(Date.parse(value.created_at))
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
    (value.protocol === "tcp" || value.protocol === "udp")
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
): Promise<Server> {
  const body = await readJSON(
    await apiFetch(
      id ? `/api/v1/servers/${encodeURIComponent(id)}` : "/api/v1/servers",
      {
        method: id ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      },
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
): Promise<Server> {
  const body = await readJSON(
    await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/ember`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...input, expected_ember_id: expectedEmberID }),
      signal: AbortSignal.timeout(35000),
    }),
  );
  if (!isServer(body))
    throw new Error(i18n.t("servers.unexpectedServerResponse"));
  return body;
}
