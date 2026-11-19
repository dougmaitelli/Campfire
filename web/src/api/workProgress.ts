import { isRecord } from "../utils/validation";

export const WorkPhase = {
  Preparing: "preparing",
  Archiving: "archiving",
  Extracting: "extracting",
  Validating: "validating",
  Uploading: "uploading",
  Downloading: "downloading",
  Restoring: "restoring",
  Committing: "committing",
} as const;
export type WorkPhase = (typeof WorkPhase)[keyof typeof WorkPhase];

/** Per-phase measurements. No counter or denominator implies operation success. */
export interface WorkProgress {
  phase: WorkPhase;
  input_bytes: number;
  output_bytes: number;
  input_total?: number;
  entries: number;
  updated_at: string;
}

export function isWorkProgress(value: unknown): value is WorkProgress {
  if (
    !isRecord(value) ||
    !Object.values(WorkPhase).some((phase) => phase === value.phase)
  )
    return false;
  const count = (n: unknown) =>
    typeof n === "number" && Number.isSafeInteger(n) && n >= 0;
  return (
    count(value.input_bytes) &&
    count(value.output_bytes) &&
    count(value.entries) &&
    (!("input_total" in value) ||
      (count(value.input_total) &&
        (value.input_total === 0 ||
          (value.input_bytes as number) <= (value.input_total as number)))) &&
    typeof value.updated_at === "string" &&
    Number.isFinite(Date.parse(value.updated_at))
  );
}
