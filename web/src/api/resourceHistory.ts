import i18n from "../i18n";
import { apiFetch, readJSON } from "./client";

export interface ResourceHistory {
  from: number;
  to: number;
  interval_seconds: number;
  samples: {
    collected_at: number;
    usage: { cpu_percent: number | null; memory_bytes: number | null };
  }[];
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
  const measurement = (input: unknown) =>
    input === null ||
    (typeof input === "number" && Number.isFinite(input) && input >= 0);
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
        object(sample.usage) &&
        measurement(sample.usage.cpu_percent) &&
        (sample.usage.memory_bytes === null ||
          timestamp(sample.usage.memory_bytes)),
    )
  ) {
    throw new Error(i18n.t("resourceHistory.unexpectedResponse"));
  }
  return value as unknown as ResourceHistory;
}
