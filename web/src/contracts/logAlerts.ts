import { isEnumValue, isRecord } from "../utils/validation";

export const LogSeverity = { Warning: "warning", Error: "error" } as const;
export type LogSeverity = (typeof LogSeverity)[keyof typeof LogSeverity];
export const LogPhase = {
  Runtime: "runtime",
  Installation: "installation",
} as const;
export type LogPhase = (typeof LogPhase)[keyof typeof LogPhase];
export const LogAction = {
  OpenFile: "open_file",
  ServerSettings: "server_settings",
} as const;
export type LogAction = (typeof LogAction)[keyof typeof LogAction];

export interface LogAlertAction {
  type: LogAction;
  path?: string;
}

export interface LogMatch {
  value: string;
  regex?: boolean;
  ignore_case?: boolean;
}

export interface LogAlert {
  title: string;
  message: string;
  severity: LogSeverity;
  phase?: LogPhase;
  any: LogMatch[];
  action?: LogAlertAction;
}

export interface DetectedLogAlert {
  id: string;
  rule_key: string;
  execution_id: string;
  phase: LogPhase;
  title: string;
  message: string;
  severity: LogSeverity;
  action?: LogAlertAction;
  detected_at: number;
  line?: string;
}

function isLogAlertAction(value: unknown): value is LogAlertAction {
  return (
    isRecord(value) &&
    isEnumValue(LogAction, value.type) &&
    (!("path" in value) || typeof value.path === "string")
  );
}

function isLogAlertText(value: Record<string, unknown>): boolean {
  return (
    typeof value.title === "string" &&
    typeof value.message === "string" &&
    isEnumValue(LogSeverity, value.severity) &&
    (!("action" in value) || isLogAlertAction(value.action))
  );
}

export function isLogAlert(value: unknown): value is LogAlert {
  return (
    isRecord(value) &&
    isLogAlertText(value) &&
    (!("phase" in value) || isEnumValue(LogPhase, value.phase)) &&
    Array.isArray(value.any) &&
    value.any.every(isLogMatch)
  );
}

export function isLogMatch(value: unknown): value is LogMatch {
  return (
    isRecord(value) &&
    typeof value.value === "string" &&
    ["regex", "ignore_case"].every(
      (key) => !(key in value) || typeof value[key] === "boolean",
    )
  );
}

export function isDetectedLogAlert(value: unknown): value is DetectedLogAlert {
  return (
    isRecord(value) &&
    isLogAlertText(value) &&
    ["id", "rule_key", "execution_id"].every(
      (key) =>
        typeof value[key] === "string" && /^[a-f0-9]{64}$/.test(value[key]),
    ) &&
    isEnumValue(LogPhase, value.phase) &&
    typeof value.detected_at === "number" &&
    Number.isSafeInteger(value.detected_at) &&
    value.detected_at > 0 &&
    (!("line" in value) || typeof value.line === "string")
  );
}
