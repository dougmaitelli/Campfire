import { apiFetch, readJSON } from "./client";

const SPARK_UPDATE_REQUEST_TIMEOUT_MS = 330000;

export interface PortAllocation {
  bind_ip: string;
  start: number;
  end: number;
  protocol: "tcp" | "udp";
}

export interface SparkSettings {
  game_address: string;
  cpu_limit: number;
  memory_mb: number;
  disk_mb: number;
  port_allocations: PortAllocation[];
}

export interface Spark extends SparkSettings {
  id: string;
  name: string;
  description: string;
  disabled: boolean;
  connected: boolean;
  agent_version?: string;
  latest_agent_version?: string;
  agent_update_available?: boolean;
  protocol?: number;
  capabilities?: string[];
  created_at: string;
}

export interface CreateSparkInput extends SparkSettings {
  name: string;
  description: string;
  disabled: boolean;
}

export function isSpark(value: unknown): value is Spark {
  return (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof value.id === "string" &&
    "name" in value &&
    typeof value.name === "string" &&
    "game_address" in value &&
    typeof value.game_address === "string" &&
    "cpu_limit" in value &&
    typeof value.cpu_limit === "number" &&
    Number.isFinite(value.cpu_limit) &&
    "memory_mb" in value &&
    Number.isSafeInteger(value.memory_mb) &&
    "disk_mb" in value &&
    Number.isSafeInteger(value.disk_mb) &&
    "port_allocations" in value &&
    Array.isArray(value.port_allocations) &&
    value.port_allocations.every(isAllocation) &&
    "description" in value &&
    typeof value.description === "string" &&
    "disabled" in value &&
    typeof value.disabled === "boolean" &&
    "connected" in value &&
    typeof value.connected === "boolean" &&
    (!("agent_version" in value) || typeof value.agent_version === "string") &&
    (!("latest_agent_version" in value) ||
      typeof value.latest_agent_version === "string") &&
    (!("agent_update_available" in value) ||
      typeof value.agent_update_available === "boolean") &&
    (!("protocol" in value) ||
      (typeof value.protocol === "number" &&
        Number.isSafeInteger(value.protocol) &&
        value.protocol > 0)) &&
    (!("capabilities" in value) ||
      (Array.isArray(value.capabilities) &&
        value.capabilities.every(
          (entry: unknown) => typeof entry === "string",
        ))) &&
    "created_at" in value &&
    typeof value.created_at === "string" &&
    Number.isFinite(Date.parse(value.created_at))
  );
}

function isAllocation(value: unknown): value is PortAllocation {
  return (
    typeof value === "object" &&
    value !== null &&
    "bind_ip" in value &&
    typeof value.bind_ip === "string" &&
    "start" in value &&
    typeof value.start === "number" &&
    Number.isInteger(value.start) &&
    "end" in value &&
    typeof value.end === "number" &&
    Number.isInteger(value.end) &&
    "protocol" in value &&
    (value.protocol === "tcp" || value.protocol === "udp")
  );
}

async function readResponse(response: Response): Promise<unknown> {
  return readJSON(response);
}

export async function listSparks(signal: AbortSignal): Promise<Spark[]> {
  const response = await apiFetch("/api/v1/sparks", { signal });
  const body = await readResponse(response);
  if (!Array.isArray(body) || !body.every(isSpark)) {
    throw new Error("Unexpected response while loading Sparks.");
  }
  return body;
}

export async function createSpark(input: CreateSparkInput): Promise<Spark> {
  const response = await apiFetch("/api/v1/sparks", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  const body = await readResponse(response);
  if (!isSpark(body)) {
    throw new Error("Unexpected response while adding the Spark.");
  }
  return body;
}

export async function updateSpark(
  id: string,
  input: Partial<Omit<CreateSparkInput, "name">>,
): Promise<Spark> {
  const response = await apiFetch(`/api/v1/sparks/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });
  const body = await readResponse(response);
  if (!isSpark(body))
    throw new Error("Unexpected response while updating the Spark.");
  return body;
}

export async function deleteSpark(id: string): Promise<void> {
  const response = await apiFetch(`/api/v1/sparks/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  if (!response.ok) await readResponse(response);
}

export async function updateSparkClient(
  id: string,
  version: string,
): Promise<void> {
  await readJSON(
    await apiFetch(`/api/v1/sparks/${encodeURIComponent(id)}/update`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ version }),
      signal: AbortSignal.timeout(SPARK_UPDATE_REQUEST_TIMEOUT_MS),
    }),
  );
}

export interface SparkEnrollment {
  token: string;
  expires_at: string;
  agent_url?: string;
  server_ca?: string;
}

export async function createSparkEnrollment(
  id: string,
): Promise<SparkEnrollment> {
  const body = await readResponse(
    await apiFetch(`/api/v1/sparks/${encodeURIComponent(id)}/enrollment`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    }),
  );
  if (
    typeof body !== "object" ||
    body === null ||
    !("token" in body) ||
    typeof body.token !== "string" ||
    !("expires_at" in body) ||
    typeof body.expires_at !== "string" ||
    ("agent_url" in body && typeof body.agent_url !== "string") ||
    ("server_ca" in body &&
      (typeof body.server_ca !== "string" || body.server_ca.length > 65536))
  ) {
    throw new Error("Unexpected enrollment response.");
  }
  return {
    token: body.token,
    expires_at: body.expires_at,
    ...("agent_url" in body ? { agent_url: body.agent_url as string } : {}),
    ...("server_ca" in body ? { server_ca: body.server_ca as string } : {}),
  };
}
