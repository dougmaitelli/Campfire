import { apiFetch, readJSON } from "./client";

export interface EmberVariable {
  name: string;
  default: string;
  required: boolean;
  editable: boolean;
  pattern?: string;
}

export interface Ember {
  id: string;
  schema: number;
  name: string;
  description: string;
  images: Record<string, string>;
  startup: string;
  work_dir: string;
  user: string;
  variables: EmberVariable[] | null;
  required_ports?: { container: number; protocol: "tcp" | "udp" }[] | null;
  origin?: {
    format: string;
    author: string;
    update_url: string;
    warnings: string[];
  };
}

export function isEmber(value: unknown): value is Ember {
  return (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    typeof value.id === "string" &&
    "schema" in value &&
    value.schema === 1 &&
    "name" in value &&
    typeof value.name === "string" &&
    "description" in value &&
    typeof value.description === "string" &&
    "images" in value &&
    typeof value.images === "object" &&
    value.images !== null &&
    !Array.isArray(value.images) &&
    Object.values(value.images).every(
      (image: unknown) => typeof image === "string",
    ) &&
    "startup" in value &&
    typeof value.startup === "string" &&
    "work_dir" in value &&
    typeof value.work_dir === "string" &&
    "user" in value &&
    typeof value.user === "string" &&
    "variables" in value &&
    (value.variables === null ||
      (Array.isArray(value.variables) && value.variables.every(isVariable))) &&
    (!("required_ports" in value) ||
      value.required_ports === null ||
      (Array.isArray(value.required_ports) &&
        value.required_ports.every(isRequiredPort)))
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
    typeof value.editable === "boolean"
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
    (value.protocol === "tcp" || value.protocol === "udp")
  );
}

export async function listEmbers(signal: AbortSignal): Promise<Ember[]> {
  const body = await readJSON(await apiFetch("/api/v1/embers", { signal }));
  if (!Array.isArray(body) || !body.every(isEmber))
    throw new Error("Unexpected Ember list response.");
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
  startupCommand: string,
  acknowledgeWarnings: boolean,
): Promise<Ember> {
  const body = await readJSON(
    await apiFetch("/api/v1/embers", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        definition,
        startup_command: startupCommand,
        acknowledge_warnings: acknowledgeWarnings,
      }),
    }),
  );
  if (!isEmber(body)) throw new Error("Unexpected Ember import response.");
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
  startup_commands: string[];
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
    !body.errors.every(isIssue) ||
    !("startup_commands" in body) ||
    !Array.isArray(body.startup_commands) ||
    !body.startup_commands.every((value: unknown) => typeof value === "string")
  )
    throw new Error("Unexpected compatibility preview response.");
  return {
    format: body.format,
    definition: body.definition,
    ember: body.ember,
    warnings: body.warnings,
    errors: body.errors,
    startup_commands: body.startup_commands,
  };
}

export async function previewEmberDefinition(
  definition: string,
  startupCommand: string,
): Promise<EmberPreview> {
  return readPreview(
    await apiFetch("/api/v1/embers/preview", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ definition, startup_command: startupCommand }),
    }),
  );
}

export async function previewEmberURL(url: string): Promise<EmberPreview> {
  return readPreview(
    await apiFetch("/api/v1/embers/preview-url", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url }),
    }),
  );
}
