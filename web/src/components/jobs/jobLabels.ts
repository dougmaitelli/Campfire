import { JobStatus, JobStep } from "../../contracts/domains";

export const jobStepLabels = {
  [JobStep.Preparing]: "serverCopy.preparing",
  [JobStep.Exporting]: "serverJobs.exporting",
  [JobStep.Copying]: "serverCopy.uploading",
  [JobStep.Importing]: "serverJobs.importing",
  [JobStep.Committing]: "serverTransferDrawer.changingPlacement",
  [JobStep.Cleanup]: "serverCopy.cleanup",
} as const satisfies Record<JobStep, string>;

export const jobStatusLabels = {
  [JobStatus.Pending]: "common.pending",
  [JobStatus.Running]: "common.running",
  [JobStatus.Failed]: "common.failed",
  [JobStatus.Cancelling]: "serverCopy.cancelling",
  [JobStatus.Completed]: "serverCopy.complete",
  [JobStatus.Cancelled]: "serverCopy.cancelled",
} as const satisfies Record<JobStatus, string>;
