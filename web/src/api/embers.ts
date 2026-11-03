import {
  ConfigParser,
  NetworkProtocol,
  VariableType,
  VariableFormat,
} from "../contracts/domains";
import {
  isEnumValue,
  isRecord,
  isNonnegativeInteger,
} from "../utils/validation";
import i18n from "../i18n";
import { isLogAlert, isLogMatch } from "../contracts/logAlerts";
import { API_REQUEST_TIMEOUT_MS, apiFetch, readJSON, apiJSON } from "./client";

import type { Ember, EmberVariable } from "../contracts/ember";
export type {
  Ember,
  EmberVariable,
  EmberVariableValidation,
  EmberConfigFile,
} from "../contracts/ember";

export function isEmber(value: unknown): value is Ember {
  return (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof value.id === "string" &&
    (!("revision" in value) || typeof value.revision === "string") &&
    "schema" in value &&
    value.schema === 1 &&
    "name" in value &&
    typeof value.name === "string" &&
    "description" in value &&
    typeof value.description === "string" &&
    (!("artUrl" in value) || typeof value.artUrl === "string") &&
    (!("updateUrl" in value) || typeof value.updateUrl === "string") &&
    "images" in value &&
    typeof value.images === "object" &&
    value.images !== null &&
    !Array.isArray(value.images) &&
    Object.values(value.images).every(
      (image: unknown) => typeof image === "string",
    ) &&
    "startup" in value &&
    typeof value.startup === "string" &&
    (!("startup_commands" in value) ||
      (typeof value.startup_commands === "object" &&
        value.startup_commands !== null &&
        !Array.isArray(value.startup_commands) &&
        Object.values(value.startup_commands).every(
          (command: unknown) => typeof command === "string",
        ))) &&
    (!("startup_shell" in value) ||
      value.startup_shell === "sh" ||
      value.startup_shell === "bash" ||
      value.startup_shell === "ash") &&
    "work_dir" in value &&
    typeof value.work_dir === "string" &&
    "user" in value &&
    typeof value.user === "string" &&
    (!("installer" in value) ||
      (isRecord(value.installer) &&
        typeof value.installer.image === "string" &&
        typeof value.installer.entrypoint === "string" &&
        typeof value.installer.script === "string")) &&
    (!("stop" in value) || typeof value.stop === "string") &&
    (!("ready" in value) || typeof value.ready === "string") &&
    (!("readiness" in value) ||
      (isRecord(value.readiness) &&
        Array.isArray(value.readiness.any) &&
        value.readiness.any.every(isLogMatch))) &&
    (!("configs" in value) ||
      (Array.isArray(value.configs) && value.configs.every(isConfigFile))) &&
    (!("log_alerts" in value) ||
      (Array.isArray(value.log_alerts) &&
        value.log_alerts.every(isLogAlert))) &&
    (!("savedata_paths" in value) ||
      (Array.isArray(value.savedata_paths) &&
        value.savedata_paths.every(
          (path: unknown) =>
            isRecord(path) &&
            typeof path.path === "string" &&
            typeof path.required === "boolean",
        ))) &&
    (!("origin" in value) ||
      (isRecord(value.origin) &&
        typeof value.origin.format === "string" &&
        typeof value.origin.author === "string" &&
        typeof value.origin.update_url === "string" &&
        (value.origin.warnings === null ||
          (Array.isArray(value.origin.warnings) &&
            value.origin.warnings.every(
              (warning: unknown) => typeof warning === "string",
            ))))) &&
    "variables" in value &&
    (value.variables === null ||
      (Array.isArray(value.variables) && value.variables.every(isVariable))) &&
    (!("required_ports" in value) ||
      value.required_ports === null ||
      (Array.isArray(value.required_ports) &&
        value.required_ports.every(isRequiredPort)))
  );
}

function isConfigFile(value: unknown): boolean {
  return (
    isRecord(value) &&
    typeof value.path === "string" &&
    isEnumValue(ConfigParser, value.parser) &&
    (value.values === null ||
      (isRecord(value.values) &&
        Object.values(value.values).every(
          (item) => typeof item === "string",
        ))) &&
    (!("create" in value) || typeof value.create === "boolean")
  );
}

function isVariable(value: unknown): value is EmberVariable {
  return (
    typeof value === "object" &&
    value !== null &&
    "name" in value &&
    typeof value.name === "string" &&
    "default" in value &&
    typeof value.default === "string" &&
    "required" in value &&
    typeof value.required === "boolean" &&
    "editable" in value &&
    typeof value.editable === "boolean" &&
    (!("hidden" in value) || typeof value.hidden === "boolean") &&
    (!("pattern" in value) || typeof value.pattern === "string") &&
    (!("validation" in value) || isVariableValidation(value.validation))
  );
}

function isVariableValidation(value: unknown): boolean {
  return (
    isRecord(value) &&
    isEnumValue(VariableType, value.type) &&
    (!("format" in value) ||
      (value.type === VariableType.String &&
        isEnumValue(VariableFormat, value.format))) &&
    (!("nullable" in value) || typeof value.nullable === "boolean") &&
    ["min", "max"].every(
      (key) =>
        !(key in value) ||
        (typeof value[key] === "number" && Number.isFinite(value[key])),
    ) &&
    ["min_length", "max_length"].every(
      (key) => !(key in value) || isNonnegativeInteger(value[key]),
    ) &&
    (!("choices" in value) ||
      (Array.isArray(value.choices) &&
        value.choices.every((choice: unknown) => typeof choice === "string")))
  );
}

function isRequiredPort(value: unknown): boolean {
  return (
    typeof value === "object" &&
    value !== null &&
    "container" in value &&
    typeof value.container === "number" &&
    Number.isInteger(value.container) &&
    "protocol" in value &&
    isEnumValue(NetworkProtocol, value.protocol) &&
    (!("description" in value) || typeof value.description === "string")
  );
}

export async function listEmbers(signal: AbortSignal): Promise<Ember[]> {
  const body = await readJSON(await apiFetch("/api/v1/embers", { signal }));
  if (!Array.isArray(body) || !body.every(isEmber))
    throw new Error(i18n.t("embers.unexpectedEmberListResponse"));
  return body;
}

export async function deleteEmber(id: string): Promise<void> {
  const response = await apiFetch(`/api/v1/embers/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  if (!response.ok) await readJSON(response);
}

export async function importEmber(
  definition: string,
  acknowledgeWarnings: boolean,
  sourceURL?: string,
  replaceID?: string,
): Promise<Ember> {
  const body = await readJSON(
    await apiJSON(
      replaceID
        ? `/api/v1/embers/${encodeURIComponent(replaceID)}`
        : "/api/v1/embers",
      {
        definition,
        acknowledge_warnings: acknowledgeWarnings,
        source_url: sourceURL,
      },
      { method: replaceID ? "PUT" : "POST" },
    ),
  );
  if (!isEmber(body))
    throw new Error(i18n.t("embers.unexpectedEmberImportResponse"));
  return body;
}

export interface ImportIssue {
  field: string;
  message: string;
  informational?: boolean;
}
export interface EmberPreview {
  format: string;
  definition: string;
  ember: Ember | null;
  warnings: ImportIssue[];
  errors: ImportIssue[];
}

function isIssue(value: unknown): value is ImportIssue {
  return (
    typeof value === "object" &&
    value !== null &&
    "field" in value &&
    typeof value.field === "string" &&
    "message" in value &&
    typeof value.message === "string" &&
    (!("informational" in value) || typeof value.informational === "boolean")
  );
}

async function readPreview(response: Response): Promise<EmberPreview> {
  const body = await readJSON(response);
  if (
    typeof body !== "object" ||
    body === null ||
    !("format" in body) ||
    typeof body.format !== "string" ||
    !("definition" in body) ||
    typeof body.definition !== "string" ||
    !("ember" in body) ||
    (body.ember !== null && !isEmber(body.ember)) ||
    !("warnings" in body) ||
    !Array.isArray(body.warnings) ||
    !body.warnings.every(isIssue) ||
    !("errors" in body) ||
    !Array.isArray(body.errors) ||
    !body.errors.every(isIssue)
  )
    throw new Error(i18n.t("embers.unexpectedCompatibilityPreviewResponse"));
  return {
    format: body.format,
    definition: body.definition,
    ember: body.ember,
    warnings: body.warnings,
    errors: body.errors,
  };
}

export async function previewEmberDefinition(
  definition: string,
  sourceURL?: string,
  signal?: AbortSignal,
): Promise<EmberPreview> {
  return readPreview(
    await apiJSON(
      "/api/v1/embers/preview",
      {
        definition,
        source_url: sourceURL,
      },
      {
        method: "POST",
        signal: signal
          ? AbortSignal.any([
              signal,
              AbortSignal.timeout(API_REQUEST_TIMEOUT_MS),
            ])
          : null,
      },
    ),
  );
}

export async function previewEmberURL(url: string): Promise<EmberPreview> {
  return readPreview(
    await apiJSON("/api/v1/embers/preview-url", { url }, { method: "POST" }),
  );
}

export interface EmberUpdate {
  id: string;
  ember_id: string;
  checked_at: number;
  available: boolean;
  error?: string | undefined;
}

function readEmberUpdate(value: unknown): EmberUpdate {
  if (
    typeof value !== "object" ||
    value === null ||
    !("ember_id" in value) ||
    typeof value.ember_id !== "string" ||
    !("checked_at" in value) ||
    typeof value.checked_at !== "number" ||
    !("available" in value) ||
    typeof value.available !== "boolean" ||
    ("error" in value && typeof value.error !== "string")
  )
    throw new Error(i18n.t("embers.unexpectedEmberUpdateResponse"));
  return {
    id: value.ember_id,
    ember_id: value.ember_id,
    checked_at: value.checked_at,
    available: value.available,
    error: "error" in value ? (value.error as string) : undefined,
  };
}

export async function listEmberUpdates(
  signal: AbortSignal,
): Promise<EmberUpdate[]> {
  const body = await readJSON(
    await apiFetch("/api/v1/embers/updates", { signal }),
  );
  if (!Array.isArray(body))
    throw new Error(i18n.t("embers.unexpectedEmberUpdatesResponse"));
  return body.map(readEmberUpdate);
}

export async function checkEmberUpdate(id: string): Promise<EmberUpdate> {
  return readEmberUpdate(
    await readJSON(
      await apiJSON(
        `/api/v1/embers/${encodeURIComponent(id)}/check-update`,
        {},
        { method: "POST", signal: AbortSignal.timeout(30000) },
      ),
    ),
  );
}

export async function downloadEmberUpdate(id: string): Promise<EmberPreview> {
  return readPreview(
    await apiJSON(
      `/api/v1/embers/${encodeURIComponent(id)}/download-update`,
      {},
      { method: "POST", signal: AbortSignal.timeout(30000) },
    ),
  );
}
