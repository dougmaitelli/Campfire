import { apiFetch, apiJSON, readJSON, API_READ_TIMEOUT_MS } from "./client";
import { JobType, JobStatus, JobStep } from "../contracts/domains";
import { isRecord, isEnumValue } from "../utils/validation";
import i18n from "../i18n";

import type { ServerJob } from "../contracts/jobs";
export type { ServerJob } from "../contracts/jobs";

export interface JobFilter {
  type?: JobType | undefined;
  status?: JobStatus | undefined;
}

export function isJob(value: unknown): value is ServerJob {
  return (
    isRecord(value) &&
    isEnumValue(JobType, value.type) &&
    isEnumValue(JobStatus, value.status) &&
    isEnumValue(JobStep, value.step) &&
    [
      "id",
      "source_server_id",
      "target_server_id",
      "source_spark_id",
      "target_spark_id",
      "source_name",
      "target_name",
      "source_spark_name",
      "target_spark_name",
      "error",
    ].every((key) => typeof value[key] === "string") &&
    ["transferred_bytes", "total_bytes", "created_at", "updated_at"].every(
      (key) =>
        typeof value[key] === "number" &&
        Number.isSafeInteger(value[key]) &&
        value[key] >= 0,
    ) &&
    (value.finished_at === null ||
      (typeof value.finished_at === "number" &&
        Number.isSafeInteger(value.finished_at) &&
        value.finished_at >= 0)) &&
    typeof value.can_retry === "boolean" &&
    typeof value.can_cancel === "boolean" &&
    Array.isArray(value.waiting_for_sparks) &&
    value.waiting_for_sparks.every(
      (id: unknown) =>
        id === value.source_spark_id || id === value.target_spark_id,
    )
  );
}

export async function listJobs(
  serverID: string | undefined,
  signal: AbortSignal,
  before?: string,
  filter: JobFilter = {},
): Promise<{ items: ServerJob[]; next: string }> {
  const query = new URLSearchParams();
  if (before) query.set("before", before);
  if (filter.type) query.set("type", filter.type);
  if (filter.status) query.set("status", filter.status);
  const path = serverID
    ? `/api/v1/servers/${encodeURIComponent(serverID)}/jobs`
    : "/api/v1/jobs";
  const value = await readJSON(
    await apiFetch(`${path}${query.size ? `?${query}` : ""}`, { signal }),
  );
  if (
    !isRecord(value) ||
    !Array.isArray(value.items) ||
    !value.items.every(isJob) ||
    typeof value.next !== "string"
  )
    throw new Error(i18n.t("serverJobs.unexpectedResponse"));
  return { items: value.items, next: value.next };
}

export async function changeJob(
  id: string,
  action: "retry" | "cancel",
): Promise<ServerJob> {
  const value = await readJSON(
    await apiJSON(
      `/api/v1/jobs/${encodeURIComponent(id)}/${action}`,
      {},
      { method: "POST", signal: AbortSignal.timeout(API_READ_TIMEOUT_MS) },
    ),
  );
  if (!isJob(value)) throw new Error(i18n.t("serverJobs.unexpectedResponse"));
  return value;
}
