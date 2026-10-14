import { readJSON } from "./client";

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
  variables: Record<string, string>;
  cpu_limit: number;
  memory_mb: number;
  disk_mb: number;
  ports: ServerPort[];
}

export interface Server extends ServerConfiguration {
  id: string;
  spark_name: string;
  ember_name: string;
  game_address: string;
  status: string;
  created_at: string;
}

function isServer(value: unknown): value is Server {
  return (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof value.id === "string" &&
    "name" in value &&
    typeof value.name === "string" &&
    "description" in value &&
    typeof value.description === "string" &&
    "game_address" in value &&
    typeof value.game_address === "string" &&
    "spark_id" in value &&
    typeof value.spark_id === "string" &&
    "spark_name" in value &&
    typeof value.spark_name === "string" &&
    "ember_id" in value &&
    typeof value.ember_id === "string" &&
    "ember_name" in value &&
    typeof value.ember_name === "string" &&
    "image" in value &&
    typeof value.image === "string" &&
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
  const body = await readJSON(await fetch("/api/v1/servers", { signal }));
  if (!Array.isArray(body) || !body.every(isServer))
    throw new Error("Unexpected server list response.");
  return body;
}

export async function saveServer(
  input: ServerConfiguration,
  id?: string,
): Promise<Server> {
  const body = await readJSON(
    await fetch(
      id ? `/api/v1/servers/${encodeURIComponent(id)}` : "/api/v1/servers",
      {
        method: id ? "PUT" : "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      },
    ),
  );
  if (!isServer(body)) throw new Error("Unexpected server response.");
  return body;
}

export async function deleteServer(id: string): Promise<void> {
  const response = await fetch(`/api/v1/servers/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  if (!response.ok) await readJSON(response);
}
