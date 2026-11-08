import { FileKind, FileOperation } from "../../contracts/domains";
import { API_READ_TIMEOUT_MS } from "../../api/client";
import i18n from "../../i18n";
import { useTranslation, Trans } from "react-i18next";
import { PrerequisiteNote } from "../status/PrerequisiteNote";
import { ActionGroup } from "../layout/ActionGroup";
import { DetailsList, DetailItem } from "../layout/DetailsList";
import { FormField } from "../forms/FormField";
import { Button } from "../forms/Button";
import { CheckboxField } from "../forms/CheckboxField";
import { InlineMessage } from "../status/InlineMessage";
import { IndeterminateProgress } from "../status/IndeterminateProgress";
import { Textarea } from "../forms/Textarea";
import { useCallback, useEffect, useRef, useState } from "react";
import type { Server } from "../../api/servers";
import {
  decodeBytes,
  canOpenFile,
  fileExtension,
  encodeBytes,
  fileLimit,
  fileTransferLimit,
  manageFiles,
  uploadLimit,
} from "../../api/files";
import type { FileEntry } from "../../api/files";
import { Panel } from "../layout/Panel";
import { InputDialog } from "../layout/InputDialog";
import { ConfirmDialog } from "../layout/ConfirmDialog";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { Table, TableRow } from "../table/Table";
import { formatBytes } from "../../utils/formatBytes";
import { EMPTY_VALUE } from "../../utils/formatDisplayValue";
import "./files.css";
import { Permission } from "../../api/permissions";
import { canPerformServer } from "../../permissions";
import { useNavigationGuard } from "../../hooks/useNavigationGuard";
import { FileDropZone } from "../files/FileDropZone";
import {
  FileUploadProgress,
  type UploadProgress,
} from "../files/FileUploadProgress";

interface Editor {
  path: string;
  text: string;
  initial: string;
  revision: string;
}

function join(directory: string, name: string) {
  return `${directory}${name}`;
}

function entryName(value: string | null): string | null {
  if (value === null) return null;
  const name = value.trim();
  if (
    !name ||
    name === "." ||
    name === ".." ||
    name.includes("/") ||
    name.includes("\\") ||
    Array.from(name).some((char) => char.charCodeAt(0) < 32)
  )
    throw new Error(
      i18n.t("serverFileBrowser.enterAFileOrFolderNameWithoutSlashesOrControl"),
    );
  return name;
}

export function ServerFileBrowser({
  server,
  canWrite: runtimeCanWrite,
  initialFile,
}: {
  server: Server;
  canWrite: boolean;
  initialFile?: string | undefined;
}) {
  const { t } = useTranslation();
  const fileActionLabels = {
    rename: t("serverFileBrowser.rename2"),
    permissions: t("serverFileBrowser.permissions"),
    move: t("serverFileBrowser.move"),
    archive: t("serverFileBrowser.archiveSelected"),
    delete: t("common.delete"),
  };
  const hasWritePermission = canPerformServer(server, "writeFiles");
  const hasDeletePermission = canPerformServer(server, "deleteFiles");
  const canWrite = runtimeCanWrite && hasWritePermission;
  const canDelete = canWrite && hasDeletePermission;
  const [directory, setDirectory] = useState("/");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [maxUploadBytes, setMaxUploadBytes] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<{
    path: string;
    message: string;
  } | null>(null);
  const [creating, setCreating] = useState<"file" | "folder" | null>(null);
  const [fileAction, setFileAction] = useState<{
    kind: "rename" | "compress" | "extract";
    entry: FileEntry;
  } | null>(null);
  const [deleting, setDeleting] = useState<FileEntry[] | null>(null);
  const [permissionsEntries, setPermissionsEntries] = useState<
    FileEntry[] | null
  >(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [bulkAction, setBulkAction] = useState<"compress" | "move" | null>(
    null,
  );
  const selectable = entries.filter(
    (entry) => entry.kind !== FileKind.Unsupported,
  );
  const selectedEntries = selectable.filter((entry) =>
    selected.includes(entry.name),
  );
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const pending = useRef(true);
  const alive = useRef(true);
  const transfer = useRef<AbortController | null>(null);
  const uploadInput = useRef<HTMLInputElement | null>(null);
  const [progress, setProgress] = useState<UploadProgress | null>(null);
  const [uploadNotice, setUploadNotice] = useState<{
    tone: "info" | "success";
    text: string;
  } | null>(null);
  const canUpload =
    canWrite &&
    !busy &&
    !loading &&
    maxUploadBytes !== null &&
    !editor &&
    !creating &&
    !fileAction &&
    !deleting &&
    !permissionsEntries &&
    !bulkAction;
  useNavigationGuard(
    true,
    busy,
    editor !== null && editor.text !== editor.initial,
  );
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      transfer.current?.abort();
    };
  }, []);
  const openFile = useCallback(
    async (path: string, signal?: AbortSignal) => {
      const result = await manageFiles(
        server.id,
        { operation: FileOperation.Read, path },
        signal,
      );
      let text: string;
      try {
        text = new TextDecoder("utf-8", { fatal: true }).decode(
          decodeBytes(result.data ?? ""),
        );
        if (text.includes("\u0000")) throw new Error("binary");
      } catch {
        throw new Error(
          i18n.t(
            "serverFileBrowser.thisFileIsBinaryUseDownloadInsteadOfEditing",
          ),
        );
      }
      if (!result.revision)
        throw new Error(i18n.t("serverFileBrowser.fileRevisionIsMissing"));
      if (!signal?.aborted && alive.current)
        setEditor({
          path: result.path,
          text,
          initial: text,
          revision: result.revision,
        });
    },
    [server.id],
  );
  const load = useCallback(
    (path: string, signal?: AbortSignal, preserveSelection = false) =>
      manageFiles(server.id, { operation: FileOperation.List, path }, signal)
        .then((result) => {
          if (!signal?.aborted && alive.current) {
            setEntries(result.entries ?? []);
            setDirectory(result.path);
            setLoadError(null);
            setSelected((current) =>
              preserveSelection
                ? current.filter((name) =>
                    result.entries?.some((entry) => entry.name === name),
                  )
                : [],
            );
          }
        })
        .catch((cause: unknown) => {
          if (!signal?.aborted && alive.current)
            setLoadError({
              path,
              message:
                cause instanceof Error
                  ? cause.message
                  : i18n.t("serverFileBrowser.unableToLoadFiles"),
            });
        }),
    [server.id],
  );
  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([
      load(
        initialFile
          ? `/${initialFile.slice(0, initialFile.lastIndexOf("/") + 1)}`
          : "/",
        controller.signal,
      ),
      initialFile
        ? openFile(initialFile, controller.signal)
        : Promise.resolve(),
      hasWritePermission
        ? uploadLimit(
            server.id,
            AbortSignal.any([
              controller.signal,
              AbortSignal.timeout(API_READ_TIMEOUT_MS),
            ]),
          ).then((limit) => {
            if (!controller.signal.aborted) setMaxUploadBytes(limit);
          })
        : Promise.resolve(),
    ])
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverFileBrowser.unableToLoadFiles"),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          pending.current = false;
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [load, server.id, hasWritePermission, initialFile, openFile]);
  async function perform(work: () => void | Promise<void>) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await work();
    } catch (cause) {
      if (alive.current)
        setError(
          cause instanceof Error
            ? cause.message
            : i18n.t("serverFileBrowser.fileOperationFailed"),
        );
    } finally {
      pending.current = false;
      if (alive.current) setBusy(false);
    }
  }
  async function open(entry: FileEntry) {
    if (entry.kind === FileKind.Directory) {
      await load(join(directory, entry.name));
      return;
    }
    if (entry.size > fileLimit)
      throw new Error(
        i18n.t("serverFileBrowser.textEditingIsLimitedTo1MiBDownloadThisFile"),
      );
    await openFile(join(directory, entry.name));
  }
  function retryListing() {
    void perform(async () => {
      setLoading(true);
      try {
        await load(loadError?.path ?? directory);
      } finally {
        if (alive.current) setLoading(false);
      }
    });
  }
  function download(entry: FileEntry) {
    if (entry.size > fileTransferLimit)
      throw new Error(i18n.t("serverFileBrowser.downloadsAreLimitedTo100GiB"));
    const anchor = document.createElement("a");
    anchor.href = `/api/v1/servers/${encodeURIComponent(server.id)}/files/download?${new URLSearchParams({ path: join(directory, entry.name) })}`;
    anchor.download = entry.name;
    anchor.click();
  }
  async function save() {
    if (!editor) return;
    const result = await manageFiles(server.id, {
      operation: FileOperation.Write,
      path: editor.path,
      data: encodeBytes(new TextEncoder().encode(editor.text)),
      revision: editor.revision,
    });
    if (!result.revision)
      throw new Error(i18n.t("serverFileBrowser.fileRevisionIsMissing"));
    setEditor({ ...editor, initial: editor.text, revision: result.revision });
  }
  async function create(folder: boolean, value: string) {
    const name = entryName(value);
    if (name === null) return;
    if (folder) {
      await manageFiles(server.id, {
        operation: FileOperation.Mkdir,
        path: join(directory, name),
      });
      await load(directory);
    } else {
      setEditor({
        path: join(directory, name),
        text: "",
        initial: "",
        revision: "absent",
      });
    }
  }
  async function rename(entry: FileEntry, value: string) {
    const name = entryName(value);
    if (name === null || name === entry.name) return;
    await manageFiles(server.id, {
      operation: FileOperation.Rename,
      path: join(directory, entry.name),
      destination: join(directory, name),
    });
    await load(directory);
  }
  async function remove(items: FileEntry[]) {
    if (pending.current || !canDelete) return;
    pending.current = true;
    setBusy(true);
    setDeleteError(null);
    let remaining = items;
    try {
      for (const entry of items) {
        await manageFiles(server.id, {
          operation: FileOperation.Delete,
          path: join(directory, entry.name),
        });
        remaining = remaining.slice(1);
      }
      setDeleting(null);
    } catch (cause) {
      setDeleting(remaining);
      setDeleteError(
        cause instanceof Error
          ? cause.message
          : i18n.t("serverFileBrowser.unableToDeleteThisEntry"),
      );
    } finally {
      await load(directory, undefined, true);
      pending.current = false;
      setBusy(false);
    }
  }
  async function uploadFiles(files: File[]) {
    if (!canUpload || pending.current || !files.length) return;
    await perform(async () => {
      setUploadNotice(null);
      for (const file of files) {
        entryName(file.name);
        if (file.size > maxUploadBytes)
          throw new Error(
            t("serverFileBrowser.uploadsAreLimitedPerFile", {
              size: formatBytes(maxUploadBytes),
            }),
          );
      }
      const controller = new AbortController();
      transfer.current = controller;
      const folder = directory;
      const totalBytes = files.reduce((total, file) => total + file.size, 0);
      let uploadedBytes = 0;
      try {
        for (const [index, file] of files.entries()) {
          controller.signal.throwIfAborted();
          if (!alive.current) break;
          setProgress({
            name: file.name,
            index: index + 1,
            count: files.length,
            completedFiles: index,
            uploadedBytes,
            totalBytes,
            phase: "uploading",
          });
          const previousBytes = uploadedBytes;
          await upload(file, controller, (transferred, phase) => {
            if (alive.current && !controller.signal.aborted)
              setProgress({
                name: file.name,
                index: index + 1,
                count: files.length,
                completedFiles: index,
                uploadedBytes: previousBytes + transferred,
                totalBytes,
                phase,
              });
          });
          uploadedBytes += file.size;
        }
        if (alive.current)
          setUploadNotice({
            tone: "success",
            text: t("fileUploads.completed", { count: files.length }),
          });
      } catch (cause) {
        if (!controller.signal.aborted) throw cause;
        if (alive.current)
          setUploadNotice({ tone: "info", text: t("fileUploads.cancelled") });
      } finally {
        transfer.current = null;
        if (alive.current) {
          setProgress(null);
          await load(folder);
        }
      }
    });
  }
  function cancelUpload() {
    if (!transfer.current || transfer.current.signal.aborted) return;
    setProgress((current) =>
      current ? { ...current, phase: "cancelling" } : null,
    );
    transfer.current.abort();
  }
  async function upload(
    file: File,
    controller: AbortController,
    onProgress: (uploaded: number, phase: "uploading" | "saving") => void,
  ) {
    if (maxUploadBytes === null)
      throw new Error(
        i18n.t(
          "serverFileBrowser.uploadLimitIsUnavailableReopenFilesToTryAgain",
        ),
      );
    if (file.size > maxUploadBytes)
      throw new Error(
        i18n.t("serverFileBrowser.uploadsAreLimitedPerFile", {
          size: formatBytes(maxUploadBytes),
        }),
      );
    const name = entryName(file.name);
    if (name === null) return;
    const target = join(directory, name);
    let uploadID: string | undefined;
    let committed = false;
    try {
      const started = await manageFiles(
        server.id,
        {
          operation: FileOperation.UploadBegin,
          path: target,
          revision: "absent",
          size: file.size,
        },
        controller.signal,
      );
      uploadID = started.upload_id;
      if (!uploadID)
        throw new Error(i18n.t("serverFileBrowser.uploadSessionIsMissing"));
      for (let offset = 0; offset < file.size; offset += fileLimit) {
        controller.signal.throwIfAborted();
        const data = encodeBytes(
          new Uint8Array(
            await file.slice(offset, offset + fileLimit).arrayBuffer(),
          ),
        );
        controller.signal.throwIfAborted();
        await manageFiles(
          server.id,
          {
            operation: FileOperation.UploadChunk,
            path: target,
            upload_id: uploadID,
            offset,
            data,
          },
          controller.signal,
        );
        onProgress(Math.min(file.size, offset + fileLimit), "uploading");
      }
      controller.signal.throwIfAborted();
      onProgress(file.size, "saving");
      await manageFiles(
        server.id,
        {
          operation: FileOperation.UploadCommit,
          path: target,
          upload_id: uploadID,
        },
        controller.signal,
      );
      committed = true;
    } finally {
      if (uploadID && !committed) {
        try {
          await manageFiles(server.id, {
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
  async function archive(entry: FileEntry, extract: boolean, value: string) {
    const destination = entryName(value);
    if (!destination) return;
    await manageFiles(server.id, {
      operation: extract ? FileOperation.Extract : FileOperation.Compress,
      path: join(directory, entry.name),
      destination: join(directory, destination),
    });
    await load(directory);
  }
  async function bulkSubmit(value: string) {
    if (!canWrite || !selectedEntries.length) return;
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    let completed = false;
    try {
      if (bulkAction === "compress") {
        const name = entryName(value);
        if (!name) return;
        await manageFiles(server.id, {
          operation: FileOperation.Compress,
          path: directory,
          paths: selectedEntries.map((entry) => join(directory, entry.name)),
          destination: join(directory, name),
        });
      } else {
        const target = await manageFiles(server.id, {
          operation: FileOperation.List,
          path: value,
        });
        const destination = target.path;
        if (destination === directory)
          throw new Error(t("serverFileBrowser.chooseDifferentFolder"));
        if (target.entries?.some((entry) => selected.includes(entry.name)))
          throw new Error(t("serverFileBrowser.destinationConflict"));
        for (const entry of selectedEntries) {
          const source = join(directory, entry.name);
          if (destination.startsWith(`${source}/`))
            throw new Error(t("serverFileBrowser.invalidMoveDestination"));
        }
        for (const entry of selectedEntries) {
          await manageFiles(server.id, {
            operation: FileOperation.Rename,
            path: join(directory, entry.name),
            destination: join(destination, entry.name),
          });
        }
      }
      completed = true;
    } finally {
      await load(directory, undefined, !completed);
      pending.current = false;
      if (alive.current) setBusy(false);
    }
  }
  return (
    <Panel
      title={t("serverFileBrowser.files")}
      leading={
        !editor && (
          <details className="file-sftp">
            <summary>{t("serverFileBrowser.sftpAccess")}</summary>
            <div className="file-sftp-content">
              <p>
                {t(
                  "serverFileBrowser.connectWithAnSFTPClientUsingTheAddressAndPort",
                )}
              </p>
              <DetailsList>
                <DetailItem label={t("serverFileBrowser.username")}>
                  <code>{server.id}</code>
                </DetailItem>
                <DetailItem label={t("serverFileBrowser.password")}>
                  {t("serverFileBrowser.yourPersonalAPIToken")}
                </DetailItem>
              </DetailsList>
              <p>
                <Trans
                  i18nKey="serverFileBrowser.createATokenFromInYourAccountMenuWithAnd"
                  t={t}
                  components={{
                    strong: <strong>{t("serverFileBrowser.apiKeys")}</strong>,
                    code: <code>{Permission.ServerView}</code>,
                    code2: <code>{Permission.ServerFilesRead}</code>,
                  }}
                />
              </p>
            </div>
          </details>
        )
      }
      headingActions={
        !editor &&
        hasWritePermission && (
          <div className="file-toolbar">
            <Button
              type="button"
              variant="primary"
              aria-label={t("serverFileBrowser.uploadFiles")}
              title={t("serverFileBrowser.uploadFiles")}
              disabled={busy || loading || !canWrite || maxUploadBytes === null}
              onClick={() => uploadInput.current?.click()}
            >
              <NavigationIcon name="upload" />
              <span className="file-toolbar-label">
                {t("serverFileBrowser.uploadFiles")}
              </span>
            </Button>
            <Button
              disabled={busy || loading || !canWrite}
              aria-label={t("serverFileBrowser.createFile")}
              title={t("serverFileBrowser.createFile")}
              onClick={() => setCreating("file")}
            >
              <NavigationIcon name="file-plus" />
              <span className="file-toolbar-label">
                {t("serverFileBrowser.createFile")}
              </span>
            </Button>
            <Button
              disabled={busy || loading || !canWrite}
              aria-label={t("serverFileBrowser.createFolder")}
              title={t("serverFileBrowser.createFolder")}
              onClick={() => setCreating("folder")}
            >
              <NavigationIcon name="folder-plus" />
              <span className="file-toolbar-label">
                {t("serverFileBrowser.createFolder")}
              </span>
            </Button>
            <input
              ref={uploadInput}
              hidden
              type="file"
              multiple
              disabled={busy || loading || !canWrite || maxUploadBytes === null}
              aria-label={t("serverFileBrowser.uploadFiles")}
              onChange={(event) => {
                const files = Array.from(event.target.files ?? []);
                event.target.value = "";
                void uploadFiles(files);
              }}
            />
          </div>
        )
      }
      footerLeft={
        editor ? (
          <Button
            disabled={busy}
            onClick={() => {
              if (
                editor.text !== editor.initial &&
                !window.confirm(
                  i18n.t("serverFileBrowser.discardUnsavedFileChanges"),
                )
              )
                return;
              setEditor(null);
              void perform(() => load(directory));
            }}
          >
            {t("serverFileBrowser.backToFiles")}
          </Button>
        ) : null
      }
      footerRight={
        editor && hasWritePermission ? (
          <Button
            variant="primary"
            disabled={busy || !canWrite}
            onClick={() => {
              void perform(save);
            }}
            loading={busy}
            loadingLabel={t("serverFileBrowser.saving")}
          >
            {t("serverFileBrowser.saveFile")}
          </Button>
        ) : null
      }
    >
      <FileDropZone
        className="panel-content"
        enabled={canUpload}
        directory={directory}
        onFiles={(files) => {
          void uploadFiles(files);
        }}
        onError={setError}
      >
        {bulkAction && (
          <InputDialog
            title={t(
              bulkAction === "compress"
                ? "serverFileBrowser.archiveSelected"
                : "serverFileBrowser.moveSelected",
            )}
            label={t(
              bulkAction === "compress"
                ? "serverFileBrowser.archiveName"
                : "serverFileBrowser.destinationFolder",
            )}
            description={t(
              bulkAction === "compress"
                ? "serverFileBrowser.archiveDescription"
                : "serverFileBrowser.moveDescription",
              { count: selectedEntries.length },
            )}
            submitLabel={t(
              bulkAction === "compress"
                ? "serverFileBrowser.archiveSelected"
                : "serverFileBrowser.move",
            )}
            initialValue={
              bulkAction === "compress" ? "archive.tar.gz" : directory
            }
            onClose={() => setBulkAction(null)}
            onSubmit={bulkSubmit}
          />
        )}
        {fileAction && (
          <InputDialog
            title={
              fileAction.kind === "rename"
                ? t("serverFileBrowser.rename2")
                : fileAction.kind === "compress"
                  ? t("serverFileBrowser.compress2")
                  : t("serverFileBrowser.extractArchive")
            }
            label={
              fileAction.kind === "rename"
                ? t("serverFileBrowser.newName")
                : fileAction.kind === "compress"
                  ? t("serverFileBrowser.archiveName")
                  : t("serverFileBrowser.newFolderName")
            }
            description={t("serverFileBrowser.in", {
              fileActionEntryName: fileAction.entry.name,
              directoryTheRoot:
                directory === "/"
                  ? t("serverFileBrowser.theRootFolder")
                  : directory,
            })}
            submitLabel={
              fileAction.kind === "rename"
                ? t("serverFileBrowser.rename2")
                : fileAction.kind === "compress"
                  ? t("serverFileBrowser.compress2")
                  : t("serverFileBrowser.extract")
            }
            initialValue={
              fileAction.kind === "rename"
                ? fileAction.entry.name
                : fileAction.kind === "compress"
                  ? `${fileAction.entry.name}.tar.gz`
                  : fileAction.entry.name.replace(
                      /\.(tar\.gz|tgz|zip|rar)$/i,
                      "",
                    )
            }
            busyContent={
              fileAction.kind === "extract" && (
                <IndeterminateProgress
                  label={t("serverFileBrowser.extracting", {
                    name: fileAction.entry.name,
                  })}
                />
              )
            }
            onClose={() => setFileAction(null)}
            onSubmit={(name) =>
              fileAction.kind === "rename"
                ? rename(fileAction.entry, name)
                : archive(fileAction.entry, fileAction.kind === "extract", name)
            }
          />
        )}
        {permissionsEntries && (
          <InputDialog
            title={t("serverFileBrowser.permissions")}
            label={t("serverFileBrowser.octalPermissions")}
            description={t("serverFileBrowser.permissionsHelp", {
              count: permissionsEntries.length,
            })}
            initialValue={
              permissionsEntries[0]?.mode !== undefined &&
              permissionsEntries.every(
                (entry) => entry.mode === permissionsEntries[0]?.mode,
              )
                ? permissionsEntries[0].mode.toString(8).padStart(3, "0")
                : ""
            }
            submitLabel={t("serverFileBrowser.savePermissions")}
            onClose={() => setPermissionsEntries(null)}
            onSubmit={async (value) => {
              if (!/^[0-7]{3}$/.test(value))
                throw new Error(t("serverFileBrowser.invalidPermissions"));
              for (const entry of permissionsEntries)
                await manageFiles(server.id, {
                  operation: FileOperation.Chmod,
                  path: join(directory, entry.name),
                  mode: Number.parseInt(value, 8),
                });
              await load(directory);
            }}
          />
        )}
        {deleting && (
          <ConfirmDialog
            title={t("serverFileBrowser.deleteSelected")}
            description={t("serverFileBrowser.deleteSelectedDescription", {
              count: deleting.length,
            })}
            confirmLabel={t("common.delete")}
            danger
            busy={busy}
            error={deleteError}
            onClose={() => setDeleting(null)}
            onConfirm={() => {
              void remove(deleting);
            }}
          />
        )}
        {creating && (
          <InputDialog
            title={
              creating === "folder"
                ? t("serverFileBrowser.newFolder")
                : t("serverFileBrowser.newFile")
            }
            label={t("common.name")}
            description={t("serverFileBrowser.createIn", {
              directoryTheRoot:
                directory === "/"
                  ? t("serverFileBrowser.theRootFolder")
                  : directory,
            })}
            submitLabel={
              creating === "folder"
                ? t("serverFileBrowser.createFolder")
                : t("serverFileBrowser.createFile")
            }
            onClose={() => setCreating(null)}
            onSubmit={(name) => create(creating === "folder", name)}
          />
        )}
        {progress && (
          <FileUploadProgress progress={progress} onCancel={cancelUpload} />
        )}
        {uploadNotice && (
          <InlineMessage tone={uploadNotice.tone}>
            {uploadNotice.text}
          </InlineMessage>
        )}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {loadError && (entries.length > 0 || editor !== null) && (
          <InlineMessage role="alert">
            {loadError.message}
            <Button disabled={busy} onClick={retryListing}>
              {t("common.tryAgain")}
            </Button>
          </InlineMessage>
        )}
        {!canWrite && (
          <PrerequisiteNote>
            {t(
              hasWritePermission
                ? "serverFileBrowser.fileModificationsAreCurrentlyUnavailableWaitForTheCurrentOperation"
                : "serverFileBrowser.readOnlyAccess",
            )}
          </PrerequisiteNote>
        )}
        {editor ? (
          <>
            <FormField label={<>{editor.path}</>}>
              <Textarea
                id="server-file-content"
                className="file-editor"
                value={editor.text}
                readOnly={!canWrite}
                disabled={busy}
                spellCheck={false}
                onChange={(event) =>
                  setEditor({ ...editor, text: event.target.value })
                }
              />
            </FormField>
            {editor.text !== editor.initial && (
              <p className="list-summary" role="status">
                {t("serverFileBrowser.unsavedChanges")}
              </p>
            )}
          </>
        ) : (
          <>
            <nav
              className="file-breadcrumbs"
              aria-label={t("serverFileBrowser.fileDirectory")}
            >
              <button
                className="file-breadcrumb"
                aria-label={t("serverFileBrowser.root")}
                title={t("serverFileBrowser.root")}
                disabled={busy}
                aria-current={directory === "/" ? "location" : undefined}
                onClick={() => {
                  void perform(() => load("/"));
                }}
              >
                <NavigationIcon name="folder" />
                {t("serverFileBrowser.root")}
              </button>
              {directory !== "/" &&
                directory
                  .split("/")
                  .filter(Boolean)
                  .map((part, index, parts) => (
                    <span
                      className="file-breadcrumb-segment"
                      key={parts.slice(0, index + 1).join("/")}
                    >
                      <span aria-hidden="true">/</span>
                      <button
                        className="file-breadcrumb"
                        disabled={busy}
                        aria-current={
                          index === parts.length - 1 ? "location" : undefined
                        }
                        onClick={() => {
                          void perform(() =>
                            load(`/${parts.slice(0, index + 1).join("/")}/`),
                          );
                        }}
                      >
                        {part}
                      </button>
                    </span>
                  ))}
            </nav>
            <Table
              className="file-list table-compact"
              empty={loading || entries.length === 0}
              loading={loading}
              error={loadError?.message}
              onRetry={retryListing}
              resource={t("serverFileBrowser.files2")}
              emptyMessage={t("serverFileBrowser.thisFolderIsEmpty")}
            >
              <thead>
                <tr>
                  {hasWritePermission && (
                    <th className="table-fit">
                      <CheckboxField
                        checked={
                          selectable.length > 0 &&
                          selectedEntries.length === selectable.length
                        }
                        disabled={busy || loading || !selectable.length}
                        onChange={(event) =>
                          setSelected(
                            event.target.checked
                              ? selectable.map((entry) => entry.name)
                              : [],
                          )
                        }
                      >
                        <span className="visually-hidden">
                          {t("serverFileBrowser.selectAll")}
                        </span>
                      </CheckboxField>
                    </th>
                  )}
                  <th className="file-icon-cell" />
                  <th className="file-name-column">{t("common.name")}</th>
                  <th className="table-fit">
                    {t("serverFileBrowser.fileType")}
                  </th>
                  <th className="table-fit">{t("common.size")}</th>
                  <th className="table-fit">
                    {t("serverFileBrowser.modified")}
                  </th>
                  <th
                    className="table-actions"
                    aria-label={t("common.actions")}
                  />
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <TableRow
                    key={entry.name}
                    className={
                      selected.includes(entry.name)
                        ? "file-selected"
                        : undefined
                    }
                    onOpen={
                      busy || !canOpenFile(entry)
                        ? undefined
                        : () => {
                            void perform(() => open(entry));
                          }
                    }
                  >
                    {hasWritePermission && (
                      <td className="table-fit">
                        <CheckboxField
                          checked={selected.includes(entry.name)}
                          disabled={busy || entry.kind === FileKind.Unsupported}
                          onChange={(event) =>
                            setSelected((current) =>
                              event.target.checked
                                ? [...current, entry.name]
                                : current.filter((name) => name !== entry.name),
                            )
                          }
                        >
                          <span className="visually-hidden">
                            {t("serverFileBrowser.selectEntry", {
                              name: entry.name,
                            })}
                          </span>
                        </CheckboxField>
                      </td>
                    )}
                    <td className="file-icon-cell">
                      <span
                        className={`file-entry-icon${entry.kind === FileKind.Directory ? " file-entry-folder" : ""}`}
                      >
                        <NavigationIcon
                          name={
                            entry.kind === FileKind.Directory
                              ? "folder"
                              : entry.kind === "file"
                                ? "file"
                                : "link"
                          }
                        />
                      </span>
                    </td>
                    <td className="file-name-column">
                      <button
                        className="file-name"
                        disabled={busy || !canOpenFile(entry)}
                        onClick={() => {
                          void perform(() => open(entry));
                        }}
                      >
                        <span className="file-entry-name">{entry.name}</span>
                        {entry.kind === FileKind.Unsupported && (
                          <span className="file-entry-meta">
                            {t("serverFileBrowser.symbolicLinkOrSpecialFile")}
                          </span>
                        )}
                      </button>
                    </td>
                    <td className="table-fit file-entry-meta">
                      {entry.kind === FileKind.Directory
                        ? t("serverFileBrowser.folderType")
                        : entry.kind === "unsupported"
                          ? t("serverFileBrowser.specialType")
                          : fileExtension(entry.name).toUpperCase() ||
                            t("serverFileBrowser.fileTypeFallback")}
                    </td>
                    <td className="table-fit file-entry-meta">
                      {entry.kind === FileKind.File
                        ? formatBytes(entry.size)
                        : EMPTY_VALUE}
                    </td>
                    <td className="table-fit file-entry-meta">
                      <time dateTime={entry.modified_at}>
                        {new Date(entry.modified_at).toLocaleString()}
                      </time>
                    </td>
                    <td className="table-actions">
                      <ActionGroup>
                        {hasWritePermission &&
                          entry.kind === FileKind.File &&
                          /\.(zip|tar\.gz|tgz|rar)$/i.test(entry.name) && (
                            <Button
                              iconOnly
                              size="small"
                              title={i18n.t("serverFileBrowser.extract")}
                              aria-label={i18n.t("serverFileBrowser.extract2", {
                                entryName: entry.name,
                              })}
                              disabled={busy || !canWrite}
                              onClick={() => {
                                setFileAction({ kind: "extract", entry });
                              }}
                            >
                              <NavigationIcon name="extract" />
                            </Button>
                          )}
                        {entry.kind === FileKind.File && (
                          <Button
                            iconOnly
                            size="small"
                            title={i18n.t("serverFileBrowser.download")}
                            disabled={busy || entry.size > fileTransferLimit}
                            onClick={() => {
                              void perform(() => download(entry));
                            }}
                            aria-label={i18n.t("serverFileBrowser.download2", {
                              entryName: entry.name,
                            })}
                          >
                            <NavigationIcon name="download" />
                          </Button>
                        )}
                      </ActionGroup>
                    </td>
                  </TableRow>
                ))}
              </tbody>
            </Table>
            {hasWritePermission && (
              <div
                className="file-selection-bar"
                role="group"
                aria-label={t("common.actions")}
              >
                <span className="file-selection-count">
                  <span className="visually-hidden">
                    {t("serverFileBrowser.selectedCount", {
                      count: selectedEntries.length,
                    })}
                  </span>
                  <span className="file-selection-full" aria-hidden="true">
                    {t("serverFileBrowser.selectedCount", {
                      count: selectedEntries.length,
                    })}
                  </span>
                  <span className="file-selection-compact" aria-hidden="true">
                    {selectedEntries.length}
                  </span>
                </span>
                <ActionGroup className="file-bulk-actions">
                  <Button
                    aria-label={fileActionLabels.rename}
                    title={fileActionLabels.rename}
                    disabled={busy || !canWrite || selectedEntries.length !== 1}
                    onClick={() => {
                      const entry = selectedEntries[0];
                      if (entry) setFileAction({ kind: "rename", entry });
                    }}
                  >
                    <NavigationIcon name="rename" />
                    <span className="file-action-label">
                      {fileActionLabels.rename}
                    </span>
                  </Button>
                  <Button
                    aria-label={fileActionLabels.permissions}
                    title={fileActionLabels.permissions}
                    disabled={busy || !canWrite || !selectedEntries.length}
                    onClick={() => setPermissionsEntries(selectedEntries)}
                  >
                    <NavigationIcon name="permissions" />
                    <span className="file-action-label">
                      {fileActionLabels.permissions}
                    </span>
                  </Button>
                  <Button
                    aria-label={fileActionLabels.move}
                    title={fileActionLabels.move}
                    disabled={busy || !canWrite || !selectedEntries.length}
                    onClick={() => setBulkAction("move")}
                  >
                    <NavigationIcon name="move" />
                    <span className="file-action-label">
                      {fileActionLabels.move}
                    </span>
                  </Button>
                  <Button
                    aria-label={fileActionLabels.archive}
                    title={fileActionLabels.archive}
                    disabled={busy || !canWrite || !selectedEntries.length}
                    onClick={() => setBulkAction("compress")}
                  >
                    <NavigationIcon name="compress" />
                    <span className="file-action-label">
                      {fileActionLabels.archive}
                    </span>
                  </Button>
                  {hasDeletePermission && (
                    <Button
                      className="file-bulk-delete"
                      aria-label={fileActionLabels.delete}
                      title={fileActionLabels.delete}
                      disabled={busy || !canDelete || !selectedEntries.length}
                      onClick={() => {
                        setDeleteError(null);
                        setDeleting(selectedEntries);
                      }}
                    >
                      <NavigationIcon name="delete" />
                      <span className="file-action-label">
                        {fileActionLabels.delete}
                      </span>
                    </Button>
                  )}
                </ActionGroup>
              </div>
            )}
          </>
        )}
      </FileDropZone>
    </Panel>
  );
}
