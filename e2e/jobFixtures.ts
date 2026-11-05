import type { ServerJob } from "../web/src/contracts/jobs";
import { JobType, JobStatus, JobStep } from "../web/src/contracts/domains";

export function jobFixture(overrides: Partial<ServerJob> = {}): ServerJob {
  return {
    id: "current-copy",
    type: JobType.Copy,
    status: JobStatus.Running,
    step: JobStep.Copying,
    source_server_id: "survival",
    target_server_id: "copy-target",
    source_spark_id: "mock-europe",
    target_spark_id: "mock-america",
    source_name: "Survival",
    target_name: "Survival (copy)",
    source_spark_name: "Europe",
    target_spark_name: "America",
    transferred_bytes: 384 * 1024 ** 2,
    total_bytes: 1024 ** 3,
    error: "",
    created_at: 1792670400,
    updated_at: 1792670400,
    finished_at: null,
    can_retry: false,
    can_cancel: true,
    waiting_for_sparks: [],
    ...overrides,
  };
}
