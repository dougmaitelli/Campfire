import type { JobStatus, JobStep } from "../../contracts/domains";

export const jobStepLabels = {
  preparing: "serverCopy.preparing",
  exporting: "serverJobs.exporting",
  copying: "serverCopy.uploading",
  importing: "serverJobs.importing",
  committing: "serverTransferDrawer.changingPlacement",
  cleanup: "serverCopy.cleanup",
} as const satisfies Record<JobStep, string>;

export const jobStatusLabels = {
  pending: "common.pending",
  running: "common.running",
  failed: "common.failed",
  cancelling: "serverCopy.cancelling",
  completed: "serverCopy.complete",
  cancelled: "serverCopy.cancelled",
} as const satisfies Record<JobStatus, string>;
