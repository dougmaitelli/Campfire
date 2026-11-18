import {
  manageFiles,
  type ExtractionStatus,
  type FileRequest,
} from "../../api/files";
import { FileKind, FileOperation } from "../../contracts/domains";
import i18n from "../../i18n";
import type { FileOutcome } from "./fileOutcomes";
import { reservedFileNames, statDestination } from "./fileDestinations";
import { publicationUnconfirmed } from "./publication";
import type { ResolveFileConflict } from "./fileConflicts";

const parentOf = (name: string) =>
  name.slice(0, Math.max(0, name.lastIndexOf("/"))) || "/";
const baseOf = (name: string) => name.split("/").filter(Boolean).at(-1) ?? "";
const publicPath = (name: string) =>
  name === "." ? "/" : `/${name.replace(/^\/+/, "")}`;
const terminal = (status: ExtractionStatus) =>
  !["running", "waiting"].includes(status.state);

export interface ExtractionResult {
  state:
    Exclude<ExtractionStatus["state"], "running" | "waiting"> | "unconfirmed";
  outcomes: FileOutcome[];
}

// The server streams directly into the destination. Only actual conflicts
// require a decision; existing directories and fresh entries need no review.
export async function extractArchive({
  id,
  source,
  destination,
  directory,
  resolve,
  signal,
}: {
  id: string;
  source: string;
  destination: string;
  directory: string;
  resolve: ResolveFileConflict;
  signal: AbortSignal;
}): Promise<ExtractionResult> {
  const existing = await statDestination(id, destination, signal);
  if (!existing) return extractNewFolder(id, source, destination, signal);
  if (existing.entry.kind !== FileKind.Directory) {
    const reserved = await reservedFileNames(id, directory, [], signal);
    const choice = await resolve(
      {
        name: destination,
        kind: FileKind.Directory,
        size: 0,
        existing: existing.entry,
        duplicate: false,
      },
      { reserved, signal },
    );
    if (!choice || choice.action === "skip")
      return {
        state: choice ? "completed" : "cancelled",
        outcomes: [
          {
            name: source,
            target: destination,
            status: choice ? "skipped" : "cancelled",
          },
        ],
      };
    destination = `${directory.replace(/\/$/, "")}/${choice.name}`;
    return extractNewFolder(id, source, destination, signal);
  }
  const operationID = crypto.randomUUID();
  const input = { path: source, destination, extraction: { id: operationID } };
  let latest: ExtractionStatus | undefined;
  let finished = false;
  let sent = false;
  async function request(
    operation: FileRequest["operation"],
    extraction: FileRequest["extraction"] = input.extraction,
    requestSignal = signal,
  ) {
    const result = await manageFiles(
      id,
      { ...input, operation, extraction },
      requestSignal,
    );
    if (!result.extraction)
      throw new Error(i18n.t("files.unexpectedFileResponse"));
    if (!latest || result.extraction.version >= latest.version)
      latest = result.extraction;
    return latest;
  }
  async function stop(): Promise<ExtractionStatus | undefined> {
    try {
      let status = await request(
        FileOperation.ExtractCancel,
        input.extraction,
        AbortSignal.timeout(10000),
      );
      for (
        let attempt = 0;
        status.remaining_entries > 0 && attempt < 20;
        attempt++
      ) {
        await delay(100);
        status = await request(
          FileOperation.ExtractStatus,
          input.extraction,
          AbortSignal.timeout(10000),
        );
      }
      return status;
    } catch {
      return latest;
    }
  }
  try {
    sent = true;
    let status: ExtractionStatus;
    try {
      status = await request(FileOperation.ExtractStart);
    } catch (cause) {
      if (!publicationUnconfirmed(cause) || signal.aborted) throw cause;
      // Recover by known ID, never by starting the extraction again.
      status = await request(
        FileOperation.ExtractStatus,
        input.extraction,
        AbortSignal.timeout(10000),
      );
    }
    while (!terminal(status) || status.remaining_entries > 0) {
      signal.throwIfAborted();
      if (status.state === "waiting" && status.conflict) {
        const conflict = status.conflict;
        const target = publicPath(conflict.destination);
        const reserved = await reservedFileNames(
          id,
          parentOf(target),
          status.outcomes
            .filter(
              (item) =>
                parentOf(publicPath(item.destination)) === parentOf(target),
            )
            .map((item) => baseOf(item.destination)),
          signal,
        );
        const choice = await resolveExtractionConflict({
          status: { ...status, conflict },
          reserved,
          resolve,
          signal,
          observe: (observationSignal) =>
            request(
              FileOperation.ExtractStatus,
              input.extraction,
              observationSignal,
            ),
        });
        if (
          !choice &&
          !signal.aborted &&
          latest &&
          (terminal(latest) || latest.conflict?.id !== conflict.id)
        ) {
          status = latest;
          continue;
        }
        if (!choice) {
          status = (await stop()) ?? status;
          finished = true;
          return extractionResult(status, source, destination);
        }
        const decision = {
          id: operationID,
          conflict_id: conflict.id,
          action: choice.action,
          ...(choice.action === "rename" ? { name: choice.name } : {}),
        };
        try {
          status = await request(FileOperation.ExtractResolve, decision);
        } catch (cause) {
          if (!publicationUnconfirmed(cause) || signal.aborted) throw cause;
          // Repeating this exact conflict token/decision is idempotent. A
          // restart or missing operation is an error, never an implicit restart.
          status = await request(
            FileOperation.ExtractResolve,
            decision,
            AbortSignal.timeout(10000),
          );
        }
      } else {
        await delay(250, signal);
        status = await request(FileOperation.ExtractStatus);
      }
    }
    finished = true;
    return extractionResult(status, source, destination);
  } catch (cause) {
    if (signal.aborted) {
      const status = await stop();
      finished = true;
      return status
        ? extractionResult(status, source, destination)
        : unconfirmedResult(source, destination);
    }
    if (sent && (latest || publicationUnconfirmed(cause))) {
      // Keep confirmed results, and identify only unresolved writes as unknown.
      const stopped = await stop();
      finished = true;
      const result = stopped
        ? extractionResult(stopped, source, destination)
        : unconfirmedResult(source, destination);
      if (!publicationUnconfirmed(cause))
        result.outcomes.push({
          name: source,
          target: destination,
          status: "failed",
          message:
            cause instanceof Error
              ? cause.message
              : i18n.t("fileExtraction.someFailed"),
        });
      return result;
    }
    throw cause;
  } finally {
    if (sent && !finished) await stop();
  }
}

// Own the prompt's lifetime and observer cleanup, not operation cancellation.
// observe uses the caller's version-aware request path so the main loop retains
// the latest snapshot when expiry or a changed conflict dismisses the prompt.
async function resolveExtractionConflict({
  status,
  reserved,
  resolve,
  signal,
  observe,
}: {
  status: ExtractionStatus & {
    conflict: NonNullable<ExtractionStatus["conflict"]>;
  };
  reserved: ReadonlySet<string>;
  resolve: ResolveFileConflict;
  signal: AbortSignal;
  observe: (signal: AbortSignal) => Promise<ExtractionStatus>;
}) {
  const conflict = status.conflict;
  const dialog = new AbortController();
  const dialogSignal = AbortSignal.any([signal, dialog.signal]);
  let observationError: Error | undefined;
  // At the advertised deadline, reconcile with the server rather than trusting
  // the browser clock. A prompt must not outlive its operation or conflict.
  const observer = (async () => {
    await delay(
      Math.min(
        2147483647,
        Math.max(0, Date.parse(status.expires_at) - Date.now()),
      ),
      dialogSignal,
    );
    while (!dialogSignal.aborted) {
      const observed = await observe(dialogSignal);
      if (terminal(observed) || observed.conflict?.id !== conflict.id) {
        dialog.abort();
        return;
      }
      await delay(1000, dialogSignal);
    }
  })().catch((cause: unknown) => {
    if (!dialogSignal.aborted) {
      observationError =
        cause instanceof Error
          ? cause
          : new Error(i18n.t("fileExtraction.someFailed"));
      dialog.abort();
    }
  });
  try {
    const choice = await resolve(
      {
        name: conflict.path,
        kind: conflict.kind,
        size: conflict.size,
        existing: conflict.existing ?? null,
        duplicate: false,
        protected: Boolean(conflict.protected),
        changed: Boolean(conflict.changed),
        message: status.message ?? "",
        expiresAt: status.expires_at,
      },
      {
        reserved,
        hasRemainingConflicts: status.pending_conflicts > 1,
        forcePrompt: Boolean(conflict.changed) || Boolean(status.message),
        signal: dialogSignal,
      },
    );
    if (observationError) throw observationError;
    return choice;
  } finally {
    dialog.abort();
    await observer;
  }
}

// A fresh directory cannot contain pre-existing member conflicts. Preserve the
// larger one-shot path: it now writes directly too, without an operation/session.
async function extractNewFolder(
  id: string,
  source: string,
  destination: string,
  signal: AbortSignal,
): Promise<ExtractionResult> {
  // Before dispatch an aborted signal means no work was requested. After
  // dispatch, aborting HTTP only stops waiting; it does not cancel Spark work.
  signal.throwIfAborted();
  try {
    await manageFiles(
      id,
      { operation: FileOperation.Extract, path: source, destination },
      signal,
    );
    return {
      state: "completed",
      outcomes: [{ name: source, target: destination, status: "extracted" }],
    };
  } catch (cause) {
    if (!signal.aborted && !publicationUnconfirmed(cause)) throw cause;
    return unconfirmedResult(source, destination);
  }
}

function unconfirmedResult(
  source: string,
  destination: string,
): ExtractionResult {
  return {
    state: "unconfirmed",
    outcomes: [
      {
        name: source,
        target: destination,
        status: "unconfirmed",
        message: i18n.t("fileExtraction.completionUnconfirmed"),
      },
    ],
  };
}

function extractionResult(
  status: ExtractionStatus,
  source: string,
  destination: string,
): ExtractionResult {
  const values: FileOutcome[] = status.outcomes.map((item) => ({
    name: item.path,
    target: publicPath(item.destination),
    status: item.status === "pending" ? "unconfirmed" : item.status,
    ...(item.message ? { message: item.message } : {}),
  }));
  if (
    ["failed", "expired"].includes(status.state) &&
    !values.some((item) => item.status === "failed")
  )
    values.push({
      name: source,
      target: "",
      status: "failed",
      message: status.message ?? i18n.t("fileExtraction.someFailed"),
    });
  const state =
    status.state === "running" ||
    status.state === "waiting" ||
    status.remaining_entries > 0
      ? "unconfirmed"
      : status.state;
  // Encountered members cannot account for the rest of a running scanner.
  // Keep confirmed member results, but explicitly mark the unknown operation.
  if (
    state === "unconfirmed" &&
    !values.some((item) => item.status === "unconfirmed")
  )
    values.push(...unconfirmedResult(source, destination).outcomes);
  return { state, outcomes: values };
}

function delay(milliseconds: number, signal?: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("Extraction cancelled.", "AbortError"));
      return;
    }
    const abort = () => {
      clearTimeout(timer);
      reject(new DOMException("Extraction cancelled.", "AbortError"));
    };
    const timer = setTimeout(() => {
      signal?.removeEventListener("abort", abort);
      resolve();
    }, milliseconds);
    signal?.addEventListener("abort", abort, { once: true });
  });
}
