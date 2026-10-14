import { readJSON } from "./client";

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
}

function isEmber(value: unknown): value is Ember {
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
  const body = await readJSON(await fetch("/api/v1/embers", { signal }));
  if (!Array.isArray(body) || !body.every(isEmber))
    throw new Error("Unexpected Ember list response.");
  return body;
}

export async function importEmber(definition: unknown): Promise<Ember> {
  const body = await readJSON(
    await fetch("/api/v1/embers", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(definition),
    }),
  );
  if (!isEmber(body)) throw new Error("Unexpected Ember import response.");
  return body;
}

export async function previewEmberURL(url: string): Promise<Ember> {
  const body = await readJSON(
    await fetch("/api/v1/embers/preview-url", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url }),
    }),
  );
  if (!isEmber(body)) throw new Error("Unexpected definition URL response.");
  return body;
}
