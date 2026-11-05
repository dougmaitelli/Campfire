import { apiJSON, readJSON, API_REQUEST_TIMEOUT_MS } from "./client";
import type { ServerPort } from "./servers";
import { isJob, type ServerJob } from "./jobs";
import i18n from "../i18n";

export async function beginTransfer(
  serverID: string,
  targetSparkID: string,
  ports: ServerPort[],
): Promise<ServerJob> {
  const result = await readJSON(
    await apiJSON(
      `/api/v1/servers/${encodeURIComponent(serverID)}/transfers`,
      { target_spark_id: targetSparkID, ports },
      { method: "POST", signal: AbortSignal.timeout(API_REQUEST_TIMEOUT_MS) },
    ),
  );
  if (!isJob(result)) throw new Error(i18n.t("serverJobs.unexpectedResponse"));
  return result;
}
