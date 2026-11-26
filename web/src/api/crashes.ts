import { CrashReason } from "../contracts/domains";
import { isRecord, isNonnegativeInteger } from "../utils/validation";

export interface CrashRecord {
  id: string;
  container_id: string;
  exited_at: number;
  exit_code: number;
  reason: CrashReason;
  console?: string;
  console_unavailable: boolean;
  console_truncated: boolean;
}

export function isCrashRecord(value: unknown): value is CrashRecord {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    /^[a-fA-F0-9]{64}$/.test(value.id) &&
    typeof value.container_id === "string" &&
    /^[a-fA-F0-9]{64}$/.test(value.container_id) &&
    isNonnegativeInteger(value.exited_at) &&
    value.exited_at > 0 &&
    value.exited_at <= 253402300799 &&
    isNonnegativeInteger(value.exit_code) &&
    value.exit_code <= 255 &&
    (value.reason === CrashReason.Unknown ||
      value.reason === CrashReason.OutOfMemory) &&
    (value.console === undefined ||
      (typeof value.console === "string" &&
        new TextEncoder().encode(value.console).length <= 16384)) &&
    typeof value.console_unavailable === "boolean" &&
    typeof value.console_truncated === "boolean"
  );
}
