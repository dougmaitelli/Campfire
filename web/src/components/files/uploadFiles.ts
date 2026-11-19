import { encodeBytes, fileLimit, manageFiles } from "../../api/files";
import {
  FileOperation,
  ExtractionAction,
  ExtractionOutcomeStatus,
  BrowserPublicationStatus,
  FileUploadPhase,
} from "../../contracts/domains";
import i18n from "../../i18n";
import { formatBytes } from "../../utils/formatBytes";
import { summarizeFileOutcomes, type FileOutcome } from "./fileOutcomes";
import type { UploadProgress } from "./uploadTypes";
import { reservedFileNames, statDestination } from "./fileDestinations";
import { normalizeFileName } from "./fileNames";
import { publicationUnconfirmed } from "./publication";
import type { ResolveFileConflict } from "./fileConflicts";

class UnconfirmedUpload extends Error {}

function uploadName(value: string): string {
  const name = normalizeFileName(value);
  if (name === null)
    throw new Error(
      i18n.t("serverFileBrowser.enterAFileOrFolderNameWithoutSlashesOrControl"),
    );
  return name;
}

// Uploads share conflict decisions/results with extraction, but retain their
// own ordered queue, per-file revisions and staged publication protocol.
export async function uploadFiles({
  id,
  directory,
  files,
  maxBytes,
  resolve,
  signal,
  onProgress,
}: {
  id: string;
  directory: string;
  files: File[];
  maxBytes: number;
  resolve: ResolveFileConflict;
  signal: AbortSignal;
  onProgress: (progress: UploadProgress) => void;
}): Promise<FileOutcome[]> {
  for (const file of files) {
    uploadName(file.name);
    if (file.size > maxBytes)
      throw new Error(
        i18n.t("serverFileBrowser.uploadsAreLimitedPerFile", {
          size: formatBytes(maxBytes),
        }),
      );
  }
  let totalBytes = files.reduce((total, file) => total + file.size, 0);
  let uploadedBytes = 0;
  const outcomes: FileOutcome[] = [];
  const seen = new Set<string>();
  try {
    // Failed metadata checks never fall back to blind publication.
    const reserved = await reservedFileNames(id, directory, [], signal);
    const occupied = new Set(reserved);
    for (const file of files) reserved.add(uploadName(file.name));
    for (const [index, file] of files.entries()) {
      let name = uploadName(file.name);
      const duplicate = seen.has(name);
      seen.add(name);
      let outcome: FileOutcome["status"] = BrowserPublicationStatus.Uploaded;
      try {
        signal.throwIfAborted();
        const info = await statDestination(id, `${directory}${name}`, signal);
        const existing = info?.entry ?? null;
        let revision = info?.revision ?? "absent";
        if (existing) occupied.add(name);
        else occupied.delete(name);
        if (existing || duplicate) {
          const futureNames = new Set(seen);
          const hasRemainingConflicts = files.slice(index + 1).some((next) => {
            const nextName = uploadName(next.name);
            const conflicts =
              occupied.has(nextName) || futureNames.has(nextName);
            futureNames.add(nextName);
            return conflicts;
          });
          const choice = await resolve(
            { name, size: file.size, existing, duplicate },
            { reserved, hasRemainingConflicts, signal },
          );
          if (!choice) break;
          if (choice.action === ExtractionAction.Skip) {
            outcomes.push({
              name: file.name,
              target: name,
              status: ExtractionOutcomeStatus.Skipped,
            });
            totalBytes -= file.size;
            continue;
          }
          if (choice.action === ExtractionAction.Rename) {
            name = uploadName(choice.name);
            revision = "absent";
            outcome = ExtractionOutcomeStatus.Renamed;
          } else {
            outcome = ExtractionOutcomeStatus.Overwritten;
          }
        }
        signal.throwIfAborted();
        reserved.add(name);
        const completedFiles = summarizeFileOutcomes(outcomes).completed;
        const previousBytes = uploadedBytes;
        const progress = {
          name,
          index: index + 1,
          count: files.length,
          completedFiles,
          uploadedBytes,
          totalBytes,
        };
        onProgress({ ...progress, phase: FileUploadPhase.Uploading });
        await uploadFile(
          id,
          `${directory}${name}`,
          file,
          revision,
          signal,
          (transferred, phase) => {
            if (!signal.aborted)
              onProgress({
                ...progress,
                uploadedBytes: previousBytes + transferred,
                phase,
              });
          },
        );
        occupied.add(name);
        uploadedBytes += file.size;
        outcomes.push({ name: file.name, target: name, status: outcome });
      } catch (cause) {
        if (cause instanceof UnconfirmedUpload) {
          outcomes.push({
            name: file.name,
            target: name,
            status: BrowserPublicationStatus.Unconfirmed,
            message: cause.message,
          });
          break;
        }
        if (signal.aborted) break;
        outcomes.push({
          name: file.name,
          target: name,
          status: ExtractionOutcomeStatus.Failed,
          message:
            cause instanceof Error
              ? cause.message
              : i18n.t("serverFileBrowser.fileOperationFailed"),
        });
        totalBytes -= file.size;
      }
    }
  } catch (cause) {
    if (!signal.aborted && files.length) {
      outcomes.push({
        name: files[0]!.name,
        target: files[0]!.name,
        status: ExtractionOutcomeStatus.Failed,
        message:
          cause instanceof Error
            ? cause.message
            : i18n.t("serverFileBrowser.fileOperationFailed"),
      });
    }
  }
  for (const file of files.slice(outcomes.length))
    outcomes.push({
      name: file.name,
      target: file.name,
      status: ExtractionOutcomeStatus.Cancelled,
    });
  return outcomes;
}

async function uploadFile(
  id: string,
  target: string,
  file: File,
  revision: string,
  signal: AbortSignal,
  onProgress: (
    uploaded: number,
    phase: FileUploadPhase.Uploading | FileUploadPhase.Saving,
  ) => void,
) {
  let uploadID: string | undefined;
  let committed = false;
  let commitStarted = false;
  try {
    const started = await manageFiles(
      id,
      {
        operation: FileOperation.UploadBegin,
        path: target,
        revision,
        size: file.size,
      },
      signal,
    );
    uploadID = started.upload_id;
    if (!uploadID)
      throw new Error(i18n.t("serverFileBrowser.uploadSessionIsMissing"));
    for (let offset = 0; offset < file.size; offset += fileLimit) {
      signal.throwIfAborted();
      const data = encodeBytes(
        new Uint8Array(
          await file.slice(offset, offset + fileLimit).arrayBuffer(),
        ),
      );
      signal.throwIfAborted();
      await manageFiles(
        id,
        {
          operation: FileOperation.UploadChunk,
          path: target,
          upload_id: uploadID,
          offset,
          data,
        },
        signal,
      );
      onProgress(
        Math.min(file.size, offset + fileLimit),
        FileUploadPhase.Uploading,
      );
    }
    signal.throwIfAborted();
    onProgress(file.size, FileUploadPhase.Saving);
    commitStarted = true;
    await manageFiles(
      id,
      {
        operation: FileOperation.UploadCommit,
        path: target,
        upload_id: uploadID,
      },
      signal,
    );
    committed = true;
  } catch (cause) {
    if (commitStarted && publicationUnconfirmed(cause))
      throw new UnconfirmedUpload(i18n.t("fileUploads.completionUnconfirmed"));
    throw cause;
  } finally {
    if (uploadID && !committed) {
      try {
        await manageFiles(id, {
          operation: FileOperation.UploadCancel,
          path: target,
          upload_id: uploadID,
        });
      } catch {
        /* Expired staging files are cleaned on the Spark. */
      }
    }
  }
}
