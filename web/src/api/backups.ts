import { readJSON } from "./client";
import type { FileEntry } from "./files";

export async function listBackups(
  id: string,
  signal: AbortSignal,
): Promise<FileEntry[]> {
  const body: unknown = await readJSON(
    await fetch(`/api/v1/servers/${encodeURIComponent(id)}/backups`, {
      signal,
    }),
  );
  if (
    typeof body !== "object" ||
    body === null ||
    !("server_id" in body) ||
    body.server_id !== id ||
    ("entries" in body && !Array.isArray(body.entries))
  )
    throw new Error("Unexpected backup response.");
  const entries: unknown[] =
    "entries" in body && Array.isArray(body.entries) ? body.entries : [];
  return entries.map((entry) => {
    if (
      typeof entry !== "object" ||
      entry === null ||
      !("name" in entry) ||
      typeof entry.name !== "string" ||
      !/^[A-Za-z0-9_-]{1,128}\.tar\.gz$/.test(entry.name) ||
      !("size" in entry) ||
      typeof entry.size !== "number" ||
      !Number.isSafeInteger(entry.size) ||
      entry.size < 0 ||
      !("modified_at" in entry) ||
      typeof entry.modified_at !== "string" ||
      !Number.isFinite(Date.parse(entry.modified_at))
    )
      throw new Error("Unexpected backup entry.");
    return {
      name: entry.name,
      kind: "file",
      size: entry.size,
      modified_at: entry.modified_at,
    };
  });
}
