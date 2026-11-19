import { manageFiles, type FileRequest } from "../../api/files";
import {
  isCompressionTerminal,
  type CompressionStatus,
} from "../../api/fileTypes";
import type { WorkProgress } from "../../api/workProgress";
import { CompressionState, FileOperation } from "../../contracts/domains";
import i18n from "../../i18n";
import { operationDelay } from "./operationDelay";
import { publicationUnconfirmed } from "./publication";

const terminal = (status: CompressionStatus) =>
  isCompressionTerminal(status.state);

/** A lost HTTP response is not successful publication. Reconcile the same
 * owner-bound operation; never retry by starting another archive. */
export async function compressArchive({
  id,
  path,
  paths,
  destination,
  signal,
  onProgress,
}: {
  id: string;
  path: string;
  paths?: string[];
  destination: string;
  signal: AbortSignal;
  onProgress: (progress: WorkProgress | undefined) => void;
}): Promise<boolean> {
  signal.throwIfAborted();
  const input = {
    path,
    ...(paths ? { paths } : {}),
    destination,
    compression: { id: crypto.randomUUID() },
  };
  let latest: CompressionStatus | undefined;
  async function request(
    operation: FileRequest["operation"],
    requestSignal: AbortSignal,
  ) {
    const result = await manageFiles(
      id,
      { ...input, operation },
      requestSignal,
    );
    if (!result.compression)
      throw new Error(i18n.t("files.unexpectedFileResponse"));
    latest = result.compression;
    onProgress(terminal(latest) ? undefined : latest.work_progress);
    return latest;
  }
  try {
    try {
      latest = await request(FileOperation.CompressStart, signal);
    } catch (cause) {
      if (signal.aborted || !publicationUnconfirmed(cause)) throw cause;
      latest = await request(FileOperation.CompressStatus, signal);
    }
    while (!terminal(latest)) {
      await operationDelay(1000, signal);
      latest = await request(FileOperation.CompressStatus, signal);
    }
  } catch (cause) {
    if (!latest && !signal.aborted && !publicationUnconfirmed(cause))
      throw cause;
    // Independent cancellation must remain possible after the viewing request
    // aborts. Wait for the writer to drain before reporting a confirmed result.
    if (!latest || !terminal(latest)) {
      try {
        latest = await request(
          FileOperation.CompressCancel,
          AbortSignal.timeout(10000),
        );
        for (let attempt = 0; !terminal(latest) && attempt < 20; attempt++) {
          await operationDelay(100);
          latest = await request(
            FileOperation.CompressStatus,
            AbortSignal.timeout(10000),
          );
        }
      } catch (cause) {
        throw new Error(i18n.t("fileCompression.unconfirmed"), { cause });
      }
    }
    if (!terminal(latest))
      throw new Error(i18n.t("fileCompression.unconfirmed"), { cause });
    if (!signal.aborted && latest.state !== CompressionState.Completed)
      throw cause;
  } finally {
    onProgress(undefined);
  }
  switch (latest.state) {
    case CompressionState.Completed:
      return true;
    case CompressionState.Cancelled:
      return false;
    case CompressionState.Failed:
      throw new Error(latest.message ?? i18n.t("fileCompression.failed"));
    case CompressionState.Running:
    case CompressionState.Cancelling:
      throw new Error(i18n.t("fileCompression.unconfirmed"));
  }
}
