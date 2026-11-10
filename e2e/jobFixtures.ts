import type { ServerJob } from "../web/src/contracts/jobs";
import { JobType, JobStatus, JobStep } from "../web/src/contracts/domains";
import type { Page } from "@playwright/test";

export async function mockJobActivity(
  page: Page,
  serverID: string,
  jobs: () => ServerJob[],
) {
  const entry = (job: ServerJob) => ({
    key: `job:${job.id}`,
    kind: "job",
    title: job.type,
    state: job.status,
    created_at: job.created_at,
    source_available: false,
    notification_count: 0,
  });
  await page.route(`**/api/v1/servers/${serverID}/activity?*`, (route) =>
    route.fulfill({ json: { items: jobs().map(entry), next: "" } }),
  );
  await page.route(
    `**/api/v1/servers/${serverID}/activity/job%3A*`,
    (route) => {
      const id = decodeURIComponent(
        new URL(route.request().url()).pathname.split("/").at(-1)!,
      ).slice(4);
      const job = jobs().find((job) => job.id === id);
      return route.fulfill(
        job
          ? {
              json: {
                ...entry(job),
                job,
                can_retry_operation: false,
              },
            }
          : { status: 404, json: { error: "Job not found" } },
      );
    },
  );
}

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
