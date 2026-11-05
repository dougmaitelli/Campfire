import type { JobType, JobStatus, JobStep } from "./domains";

export interface ServerJob {
  id: string;
  type: JobType;
  status: JobStatus;
  step: JobStep;
  source_server_id: string;
  target_server_id: string;
  source_spark_id: string;
  target_spark_id: string;
  source_name: string;
  target_name: string;
  source_spark_name: string;
  target_spark_name: string;
  transferred_bytes: number;
  total_bytes: number;
  error: string;
  created_at: number;
  updated_at: number;
  finished_at: number | null;
  can_retry: boolean;
  can_cancel: boolean;
  waiting_for_sparks: string[];
}
