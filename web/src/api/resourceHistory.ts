import i18n from "../i18n";
import { apiFetch, readJSON } from "./client";
import { isResourceUsage, type ResourceUsage } from "./monitoring";

export interface ResourceHistory {
  from: number;
  to: number;
  interval_seconds: number;
  samples: { collected_at: number; usage: ResourceUsage }[];
}

export async function readResourceHistory(
  serverID: string,
  hours: number,
  signal: AbortSignal,
): Promise<ResourceHistory> {
  const to = Math.floor(Date.now() / 1000);
  const query = new URLSearchParams({
    from: String(to - hours * 3600),
    to: String(to),
    limit: "240",
  });
  const value: unknown = await readJSON(
    await apiFetch(
      `/api/v1/servers/${encodeURIComponent(serverID)}/metrics/history?${query}`,
      { signal },
    ),
  );
  const object = (input: unknown): input is Record<string, unknown> =>
    typeof input === "object" && input !== null;
  const timestamp = (input: unknown): input is number =>
    typeof input === "number" && Number.isSafeInteger(input) && input >= 0;
  if (
    !object(value) ||
    !timestamp(value.from) ||
    !timestamp(value.to) ||
    value.to <= value.from ||
    !timestamp(value.interval_seconds) ||
    value.interval_seconds < 1 ||
    !Array.isArray(value.samples) ||
    !value.samples.every(
      (sample: unknown) =>
        object(sample) &&
        timestamp(sample.collected_at) &&
        isResourceUsage(sample.usage),
    )
  ) {
    throw new Error(i18n.t("resourceHistory.unexpectedResponse"));
  }
  return value as unknown as ResourceHistory;
}
