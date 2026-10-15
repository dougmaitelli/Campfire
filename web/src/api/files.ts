import { apiFetch, readJSON } from "./client";

export const fileLimit = 1024 * 1024;

export interface FileEntry {
  name: string;
  kind: "file" | "directory" | "unsupported";
  size: number;
  modified_at: string;
}
export interface FileResult {
  server_id: string;
  path: string;
  entries?: FileEntry[];
  data?: string;
  revision?: string;
}
export interface FileRequest {
  operation: "list" | "read" | "write" | "mkdir" | "rename" | "delete";
  path: string;
  destination?: string;
  data?: string;
  revision?: string;
}

function isEntry(value: unknown): value is FileEntry {
  return (
    typeof value === "object" &&
    value !== null &&
    "name" in value &&
    typeof value.name === "string" &&
    "kind" in value &&
    ["file", "directory", "unsupported"].includes(String(value.kind)) &&
    "size" in value &&
    typeof value.size === "number" &&
    Number.isSafeInteger(value.size) &&
    "modified_at" in value &&
    typeof value.modified_at === "string" &&
    Number.isFinite(Date.parse(value.modified_at))
  );
}

function isResult(value: unknown): value is FileResult {
  return (
    typeof value === "object" &&
    value !== null &&
    "server_id" in value &&
    typeof value.server_id === "string" &&
    "path" in value &&
    typeof value.path === "string" &&
    (!("entries" in value) ||
      (Array.isArray(value.entries) && value.entries.every(isEntry))) &&
    (!("data" in value) || typeof value.data === "string") &&
    (!("revision" in value) || typeof value.revision === "string")
  );
}

export async function manageFiles(
  id: string,
  input: FileRequest,
  signal?: AbortSignal,
): Promise<FileResult> {
  const body = await readJSON(
    await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/files`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(15000)])
        : AbortSignal.timeout(15000),
    }),
  );
  if (!isResult(body) || body.server_id !== id)
    throw new Error("Unexpected file response.");
  return body;
}

export function encodeBytes(bytes: Uint8Array): string {
  if (bytes.length > fileLimit)
    throw new Error("Transfers are limited to 1 MiB per file.");
  let binary = "";
  for (let index = 0; index < bytes.length; index += 8192)
    binary += String.fromCharCode(...bytes.subarray(index, index + 8192));
  return btoa(binary);
}

export function decodeBytes(value: string): Uint8Array<ArrayBuffer> {
  const binary = atob(value);
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}
