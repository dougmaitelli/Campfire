import i18n from "../i18n";
import { apiFetch, readJSON } from "./client";

export const fileLimit = 1024 * 1024;
export const fileTransferLimit = 100 * 1024 ** 3;

export async function uploadLimit(
  id: string,
  signal: AbortSignal,
): Promise<number> {
  const body = await readJSON(
    await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/files/limits`, {
      signal,
    }),
  );
  if (
    typeof body !== "object" ||
    body === null ||
    !("max_upload_bytes" in body) ||
    typeof body.max_upload_bytes !== "number" ||
    !Number.isSafeInteger(body.max_upload_bytes) ||
    body.max_upload_bytes < fileLimit ||
    body.max_upload_bytes > fileTransferLimit
  )
    throw new Error(i18n.t("files.unexpectedUploadLimitResponse"));
  return body.max_upload_bytes;
}

export interface FileEntry {
  mode?: number;
  name: string;
  kind: "file" | "directory" | "unsupported";
  size: number;
  modified_at: string;
}

export function fileExtension(name: string): string {
  return (
    /\.(tar\.gz|tar\.bz2|tar\.xz|[^.]+)$/i.exec(name)?.[1]?.toLowerCase() ?? ""
  );
}

// Filename hints avoid fetching known binary formats just to reject them.
// Unknown formats still go through the editor's UTF-8 and NUL checks.
export function canOpenFile(entry: FileEntry): boolean {
  if (entry.kind === "directory") return true;
  if (entry.kind !== "file" || entry.size > fileLimit) return false;
  return !new Set([
    "jar",
    "class",
    "zip",
    "gz",
    "tgz",
    "tar",
    "tar.gz",
    "tar.bz2",
    "tar.xz",
    "bz2",
    "xz",
    "7z",
    "rar",
    "zst",
    "png",
    "jpg",
    "jpeg",
    "gif",
    "webp",
    "ico",
    "bmp",
    "pdf",
    "exe",
    "dll",
    "so",
    "bin",
    "dat",
    "mca",
    "mcr",
    "nbt",
    "db",
    "sqlite",
    "sqlite3",
    "ogg",
    "mp3",
    "wav",
    "mp4",
    "webm",
    "woff",
    "woff2",
    "ttf",
  ]).has(fileExtension(entry.name));
}
export interface FileResult {
  server_id: string;
  path: string;
  entries?: FileEntry[];
  data?: string;
  revision?: string;
  upload_id?: string;
}
export interface FileRequest {
  operation:
    | "list"
    | "read"
    | "write"
    | "mkdir"
    | "rename"
    | "delete"
    | "chmod"
    | "upload.begin"
    | "upload.chunk"
    | "upload.commit"
    | "upload.cancel"
    | "compress"
    | "extract";
  path: string;
  mode?: number;
  paths?: string[];
  destination?: string;
  data?: string;
  revision?: string;
  upload_id?: string;
  offset?: number;
  size?: number;
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
    value.path.startsWith("/") &&
    (!("entries" in value) ||
      (Array.isArray(value.entries) && value.entries.every(isEntry))) &&
    (!("data" in value) || typeof value.data === "string") &&
    (!("revision" in value) || typeof value.revision === "string") &&
    (!("upload_id" in value) || typeof value.upload_id === "string")
  );
}

export async function manageFiles(
  id: string,
  input: FileRequest,
  signal?: AbortSignal,
): Promise<FileResult> {
  const timeout =
    input.operation === "compress" || input.operation === "extract"
      ? 30 * 60 * 1000
      : 15000;
  const body = await readJSON(
    await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}/files`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(timeout)])
        : AbortSignal.timeout(timeout),
    }),
  );
  if (
    !isResult(body) ||
    body.server_id !== id ||
    (input.operation === "list" && !body.path.endsWith("/"))
  )
    throw new Error(i18n.t("files.unexpectedFileResponse"));
  return body;
}

export function encodeBytes(bytes: Uint8Array): string {
  if (bytes.length > fileLimit)
    throw new Error(
      i18n.t("files.textEditsAndIndividualUploadChunksAreLimitedTo1"),
    );
  let binary = "";
  for (let index = 0; index < bytes.length; index += 8192)
    binary += String.fromCharCode(...bytes.subarray(index, index + 8192));
  return btoa(binary);
}

export function decodeBytes(value: string): Uint8Array<ArrayBuffer> {
  const binary = atob(value);
  return Uint8Array.from(binary, (char) => char.charCodeAt(0));
}
