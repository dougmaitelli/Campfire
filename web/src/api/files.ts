import { RequestTimeout } from "../config/policies";
import { isEnumValue } from "../utils/validation";
import { FileKind, FileOperation } from "../contracts/domains";
import i18n from "../i18n";
import { apiFetch, readJSON, apiJSON } from "./client";
import { extractionStates, extractionOutcomeStatuses } from "./fileTypes";
import type {
  FileEntry,
  FileRequest,
  FileResult,
  ExtractionStatus,
} from "./fileTypes";
export type {
  FileEntry,
  FileRequest,
  FileResult,
  ExtractionStatus,
  ExtractionConflict,
  ExtractionChoice,
  ExtractionOutcome,
  PublicationStatus,
} from "./fileTypes";

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

export function fileExtension(name: string): string {
  return (
    /\.(tar\.gz|tar\.bz2|tar\.xz|[^.]+)$/i.exec(name)?.[1]?.toLowerCase() ?? ""
  );
}

// Filename hints avoid fetching known binary formats just to reject them.
// Unknown formats still go through the editor's UTF-8 and NUL checks.
export function canOpenFile(entry: FileEntry): boolean {
  if (entry.kind === FileKind.Directory) return true;
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
function isEntry(value: unknown): value is FileEntry {
  return (
    typeof value === "object" &&
    value !== null &&
    "name" in value &&
    typeof value.name === "string" &&
    "kind" in value &&
    isEnumValue(FileKind, value.kind) &&
    "size" in value &&
    typeof value.size === "number" &&
    Number.isSafeInteger(value.size) &&
    "modified_at" in value &&
    typeof value.modified_at === "string" &&
    Number.isFinite(Date.parse(value.modified_at))
  );
}

function isExtraction(value: unknown): value is ExtractionStatus {
  if (typeof value !== "object" || value === null) return false;
  const v = value as Record<string, unknown>;
  if (
    typeof v.id !== "string" ||
    !v.id ||
    typeof v.version !== "number" ||
    !Number.isSafeInteger(v.version) ||
    v.version < 0 ||
    typeof v.state !== "string" ||
    !extractionStates.some((state) => state === v.state) ||
    typeof v.expires_at !== "string" ||
    !Number.isFinite(Date.parse(v.expires_at)) ||
    typeof v.pending_conflicts !== "number" ||
    !Number.isSafeInteger(v.pending_conflicts) ||
    v.pending_conflicts < 0 ||
    typeof v.remaining_entries !== "number" ||
    !Number.isSafeInteger(v.remaining_entries) ||
    v.remaining_entries < v.pending_conflicts ||
    !Array.isArray(v.outcomes) ||
    !v.outcomes.every(isOutcome) ||
    ("message" in v && typeof v.message !== "string")
  )
    return false;
  if (v.pending_conflicts === 0) return !("conflict" in v);
  if (typeof v.conflict !== "object" || v.conflict === null) return false;
  const conflict = v.conflict as Record<string, unknown>;
  return (
    typeof conflict.id === "string" &&
    conflict.id.length > 0 &&
    typeof conflict.path === "string" &&
    typeof conflict.destination === "string" &&
    (conflict.kind === FileKind.File || conflict.kind === FileKind.Directory) &&
    typeof conflict.size === "number" &&
    Number.isSafeInteger(conflict.size) &&
    conflict.size >= 0 &&
    (!("existing" in conflict) || isEntry(conflict.existing)) &&
    (!("changed" in conflict) || typeof conflict.changed === "boolean") &&
    (!("protected" in conflict) || typeof conflict.protected === "boolean")
  );
}
function isOutcome(
  value: unknown,
): value is ExtractionStatus["outcomes"][number] {
  return (
    typeof value === "object" &&
    value !== null &&
    "path" in value &&
    typeof value.path === "string" &&
    "destination" in value &&
    typeof value.destination === "string" &&
    "status" in value &&
    typeof value.status === "string" &&
    extractionOutcomeStatuses.some((status) => status === value.status) &&
    (!("message" in value) || typeof value.message === "string")
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
    (!("upload_id" in value) || typeof value.upload_id === "string") &&
    (!("extraction" in value) || isExtraction(value.extraction))
  );
}

export async function manageFiles(
  id: string,
  input: FileRequest,
  signal?: AbortSignal,
): Promise<FileResult> {
  const timeout =
    input.operation === FileOperation.Compress ||
    input.operation === FileOperation.Extract ||
    input.operation === FileOperation.ExtractStart
      ? RequestTimeout.Archive
      : RequestTimeout.Read;
  const body = await readJSON(
    await apiJSON(`/api/v1/servers/${encodeURIComponent(id)}/files`, input, {
      method: "POST",
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(timeout)])
        : AbortSignal.timeout(timeout),
    }),
  );
  if (
    !isResult(body) ||
    body.server_id !== id ||
    (input.operation === FileOperation.List && !body.path.endsWith("/")) ||
    (input.operation.startsWith("extract.") &&
      (!body.extraction || body.extraction.id !== input.extraction?.id))
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
