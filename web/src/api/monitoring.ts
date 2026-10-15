import { apiFetch, readJSON } from "./client";
import { isSpark, type Spark } from "./sparks";

export interface ResourceUsage {
  server_id: string;
  state: string;
  cpu_percent: number | null;
  memory_bytes: number | null;
  disk_bytes: number | null;
  sampled_at: number;
  disk_sampled_at: number;
  sample_age_seconds: number;
  disk_age_seconds: number;
  container_error: string;
  disk_error: string;
  disk_quota_enforced: boolean;
  disk_quota_error: string;
}
export interface MonitoredServer {
  id: string;
  name: string;
  spark_id: string;
  spark_name: string;
  cpu_limit: number;
  memory_mb: number;
  disk_mb: number;
  received_at: number;
  usage: ResourceUsage;
}
export interface MonitoringSnapshot {
  collected_at: number;
  incomplete: boolean;
  servers: MonitoredServer[];
  sparks?: Spark[];
}
function object(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
function nonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}
function integer(value: unknown) {
  return nonnegative(value) && Number.isSafeInteger(value);
}
function nullable(value: unknown) {
  return value === null || nonnegative(value);
}
export function isResourceUsage(value: unknown): value is ResourceUsage {
  return (
    object(value) &&
    typeof value.server_id === "string" &&
    typeof value.state === "string" &&
    nullable(value.cpu_percent) &&
    (value.memory_bytes === null || integer(value.memory_bytes)) &&
    (value.disk_bytes === null || integer(value.disk_bytes)) &&
    [
      "sampled_at",
      "disk_sampled_at",
      "sample_age_seconds",
      "disk_age_seconds",
    ].every((key) => integer(value[key])) &&
    typeof value.container_error === "string" &&
    typeof value.disk_error === "string" &&
    typeof value.disk_quota_enforced === "boolean" &&
    typeof value.disk_quota_error === "string"
  );
}
function isMonitoredServer(value: unknown): value is MonitoredServer {
  return (
    object(value) &&
    ["id", "name", "spark_id", "spark_name"].every(
      (key) => typeof value[key] === "string",
    ) &&
    nonnegative(value.cpu_limit) &&
    integer(value.memory_mb) &&
    integer(value.disk_mb) &&
    integer(value.received_at) &&
    isResourceUsage(value.usage) &&
    value.usage.server_id === value.id
  );
}
export async function readMonitoring(
  signal: AbortSignal,
): Promise<MonitoringSnapshot> {
  const value = await readJSON(
    await apiFetch("/api/v1/monitoring", { signal }),
  );
  if (
    !object(value) ||
    !integer(value.collected_at) ||
    typeof value.incomplete !== "boolean" ||
    !Array.isArray(value.servers) ||
    !value.servers.every(isMonitoredServer) ||
    (value.sparks !== undefined &&
      (!Array.isArray(value.sparks) || !value.sparks.every(isSpark)))
  )
    throw new Error("Unexpected monitoring response.");
  return value as unknown as MonitoringSnapshot;
}

export function containerFresh(
  usage: ResourceUsage | undefined,
  elapsed: number,
) {
  return (
    usage !== undefined &&
    usage.sampled_at > 0 &&
    usage.sample_age_seconds + elapsed <= 15 &&
    !usage.container_error
  );
}
export function diskFresh(usage: ResourceUsage | undefined, elapsed: number) {
  return (
    usage !== undefined &&
    usage.disk_sampled_at > 0 &&
    usage.disk_age_seconds + elapsed <= 90 &&
    !usage.disk_error
  );
}
export function bytes(value: number) {
  if (value < 1024) return `${value.toLocaleString()} B`;
  const units = ["KiB", "MiB", "GiB", "TiB", "PiB"];
  let amount = value / 1024;
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024;
    index++;
  }
  return `${amount.toLocaleString(undefined, { maximumFractionDigits: 1 })} ${units[index] ?? "PiB"}`;
}
