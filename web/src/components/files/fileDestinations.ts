import { APIError } from "../../api/client";
import { manageFiles } from "../../api/files";
import { FileOperation } from "../../contracts/domains";
import i18n from "../../i18n";

export async function statDestination(
  id: string,
  path: string,
  signal?: AbortSignal,
) {
  try {
    const result = await manageFiles(
      id,
      { operation: FileOperation.Stat, path },
      signal,
    );
    signal?.throwIfAborted();
    const entry = result.entries?.[0];
    if (!result.revision || !entry || result.entries?.length !== 1)
      throw new Error(i18n.t("fileUploads.metadataUnavailable"));
    return { entry, revision: result.revision };
  } catch (cause) {
    if (cause instanceof APIError && cause.status === 404) return null;
    throw cause;
  }
}

export async function reservedFileNames(
  id: string,
  directory: string,
  queued: Iterable<string>,
  signal?: AbortSignal,
) {
  const result = await manageFiles(
    id,
    { operation: FileOperation.List, path: directory },
    signal,
  );
  signal?.throwIfAborted();
  return new Set([
    ...(result.entries ?? []).map((entry) => entry.name),
    ...queued,
  ]);
}
