import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { PrerequisiteNote } from "../components/status/PrerequisiteNote";
import { ActionGroup } from "../components/layout/ActionGroup";
import { DetailsList, DetailItem } from "../components/layout/DetailsList";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { CheckboxField } from "../components/forms/CheckboxField";
import { InlineMessage } from "../components/status/InlineMessage";
import { Textarea } from "../components/forms/Textarea";
import { useCallback, useEffect, useRef, useState } from "react";
import type { Server } from "../api/servers";
import {
  decodeBytes,
  canOpenFile,
  fileExtension,
  encodeBytes,
  fileLimit,
  fileTransferLimit,
  manageFiles,
  uploadLimit,
} from "../api/files";
import type { FileEntry } from "../api/files";
import { Drawer } from "../components/layout/Drawer";
import { InputDialog } from "../components/layout/InputDialog";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { Table, TableRow } from "../components/table/Table";
import { bytes } from "../api/monitoring";
import { EMPTY_VALUE } from "../utils/formatDisplayValue";
import "../components/servers/files.css";
import { Permission } from "../api/permissions";

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
      i18n.t("serverFilesDrawer.enterAFileOrFolderNameWithoutSlashesOrControl"),
    );
  return name;
}

export function ServerFilesDrawer({
  server,
  canWrite: runtimeCanWrite,
  onClose,
  embedded = false,
}: {
  server: Server;
  canWrite: boolean;
  onClose: () => void;
  embedded?: boolean;
}) {
  const { t } = useTranslation();
  const hasWritePermission = server.permissions.includes(
    Permission.ServerFilesWrite,
  );
  const canWrite = runtimeCanWrite && hasWritePermission;
  const canDelete =
    canWrite && server.permissions.includes(Permission.ServerFilesDelete);
  const [directory, setDirectory] = useState("/");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [maxUploadBytes, setMaxUploadBytes] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
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
  const selectable = entries.filter((entry) => entry.kind !== "unsupported");
  const selectedEntries = selectable.filter((entry) =>
    selected.includes(entry.name),
  );
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const pending = useRef(true);
  const alive = useRef(true);
  const transfer = useRef<AbortController | null>(null);
  const uploadInput = useRef<HTMLInputElement | null>(null);
  const [progress, setProgress] = useState<string | null>(null);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      transfer.current?.abort();
    };
  }, []);
  const load = useCallback(
    async (path: string, signal?: AbortSignal, preserveSelection = false) => {
      const result = await manageFiles(
        server.id,
        { operation: "list", path },
        signal,
      );
      if (!signal?.aborted && alive.current) {
        setEntries(result.entries ?? []);
        setDirectory(result.path);
        setSelected((current) =>
          preserveSelection
            ? current.filter((name) =>
                result.entries?.some((entry) => entry.name === name),
              )
            : [],
        );
      }
    },
    [server.id],
  );
  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([
      load("/", controller.signal),
      hasWritePermission
        ? uploadLimit(
            server.id,
            AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
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
              : i18n.t("serverFilesDrawer.unableToLoadFiles"),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          pending.current = false;
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [load, server.id, hasWritePermission]);
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
            : i18n.t("serverFilesDrawer.fileOperationFailed"),
        );
    } finally {
      pending.current = false;
      if (alive.current) setBusy(false);
    }
  }
  function close() {
    if (
      editor &&
      editor.text !== editor.initial &&
      !window.confirm(i18n.t("serverFilesDrawer.discardUnsavedFileChanges"))
    )
      return;
    onClose();
  }
  async function open(entry: FileEntry) {
    if (entry.kind === "directory") {
      await load(join(directory, entry.name));
      return;
    }
    if (entry.size > fileLimit)
      throw new Error(
        i18n.t("serverFilesDrawer.textEditingIsLimitedTo1MiBDownloadThisFile"),
      );
    const result = await manageFiles(server.id, {
      operation: "read",
      path: join(directory, entry.name),
    });
    let text: string;
    try {
      text = new TextDecoder("utf-8", { fatal: true }).decode(
        decodeBytes(result.data ?? ""),
      );
      if (text.includes("\u0000")) throw new Error("binary");
    } catch {
      throw new Error(
        i18n.t("serverFilesDrawer.thisFileIsBinaryUseDownloadInsteadOfEditing"),
      );
    }
    if (!result.revision)
      throw new Error(i18n.t("serverFilesDrawer.fileRevisionIsMissing"));
    setEditor({
      path: result.path,
      text,
      initial: text,
      revision: result.revision,
    });
  }
  function download(entry: FileEntry) {
    if (entry.size > fileTransferLimit)
      throw new Error(i18n.t("serverFilesDrawer.downloadsAreLimitedTo100GiB"));
    const anchor = document.createElement("a");
    anchor.href = `/api/v1/servers/${encodeURIComponent(server.id)}/files/download?${new URLSearchParams({ path: join(directory, entry.name) })}`;
    anchor.download = entry.name;
    anchor.click();
  }
  async function save() {
    if (!editor) return;
    const result = await manageFiles(server.id, {
      operation: "write",
      path: editor.path,
      data: encodeBytes(new TextEncoder().encode(editor.text)),
      revision: editor.revision,
    });
    if (!result.revision)
      throw new Error(i18n.t("serverFilesDrawer.fileRevisionIsMissing"));
    setEditor({ ...editor, initial: editor.text, revision: result.revision });
  }
  async function create(folder: boolean, value: string) {
    const name = entryName(value);
    if (name === null) return;
    if (folder) {
      await manageFiles(server.id, {
        operation: "mkdir",
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
      operation: "rename",
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
          operation: "delete",
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
          : i18n.t("serverFilesDrawer.unableToDeleteThisEntry"),
      );
    } finally {
      await load(directory, undefined, true).catch(() => {});
      pending.current = false;
      setBusy(false);
    }
  }
  async function upload(file: File) {
    if (maxUploadBytes === null)
      throw new Error(
        i18n.t(
          "serverFilesDrawer.uploadLimitIsUnavailableReopenFilesToTryAgain",
        ),
      );
    if (file.size > maxUploadBytes)
      throw new Error(
        i18n.t("serverFilesDrawer.uploadsAreLimitedToMiBPerFile", {
          toLocaleString: (maxUploadBytes / 1024 ** 2).toLocaleString(),
        }),
      );
    const name = entryName(file.name);
    if (name === null) return;
    const controller = new AbortController();
    transfer.current = controller;
    const target = join(directory, name);
    let uploadID: string | undefined;
    let committed = false;
    try {
      const started = await manageFiles(
        server.id,
        {
          operation: "upload.begin",
          path: target,
          revision: "absent",
          size: file.size,
        },
        controller.signal,
      );
      uploadID = started.upload_id;
      if (!uploadID)
        throw new Error(i18n.t("serverFilesDrawer.uploadSessionIsMissing"));
      for (let offset = 0; offset < file.size; offset += fileLimit) {
        if (alive.current)
          setProgress(
            i18n.t("serverFilesDrawer.uploading", {
              fileName: file.name,
              MathFloor100: Math.floor((offset / file.size) * 100),
            }),
          );
        const data = encodeBytes(
          new Uint8Array(
            await file.slice(offset, offset + fileLimit).arrayBuffer(),
          ),
        );
        await manageFiles(
          server.id,
          {
            operation: "upload.chunk",
            path: target,
            upload_id: uploadID,
            offset,
            data,
          },
          controller.signal,
        );
      }
      await manageFiles(
        server.id,
        { operation: "upload.commit", path: target, upload_id: uploadID },
        controller.signal,
      );
      committed = true;
    } finally {
      if (uploadID && !committed) {
        try {
          await manageFiles(server.id, {
            operation: "upload.cancel",
            path: target,
            upload_id: uploadID,
          });
        } catch {
          /* Expired staging files are cleaned on the Spark. */
        }
      }
      transfer.current = null;
      if (alive.current) setProgress(null);
    }
    await load(directory);
  }
  async function archive(entry: FileEntry, extract: boolean, value: string) {
    const destination = entryName(value);
    if (!destination) return;
    await manageFiles(server.id, {
      operation: extract ? "extract" : "compress",
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
          operation: "compress",
          path: directory,
          paths: selectedEntries.map((entry) => join(directory, entry.name)),
          destination: join(directory, name),
        });
      } else {
        const target = await manageFiles(server.id, {
          operation: "list",
          path: value,
        });
        const destination = target.path;
        if (destination === directory)
          throw new Error(t("serverFilesDrawer.chooseDifferentFolder"));
        if (target.entries?.some((entry) => selected.includes(entry.name)))
          throw new Error(t("serverFilesDrawer.destinationConflict"));
        for (const entry of selectedEntries) {
          const source = join(directory, entry.name);
          if (destination.startsWith(`${source}/`))
            throw new Error(t("serverFilesDrawer.invalidMoveDestination"));
        }
        for (const entry of selectedEntries) {
          await manageFiles(server.id, {
            operation: "rename",
            path: join(directory, entry.name),
            destination: join(destination, entry.name),
          });
        }
      }
      completed = true;
    } finally {
      await load(directory, undefined, !completed).catch(() => {});
      pending.current = false;
      if (alive.current) setBusy(false);
    }
  }
  return (
    <Drawer
      embedded={embedded}
      unsavedChanges={editor !== null && editor.text !== editor.initial}
      title={t("serverFilesDrawer.files")}
      leading={
        !editor && (
          <details className="file-sftp">
            <summary>{t("serverFilesDrawer.sftpAccess")}</summary>
            <div className="file-sftp-content">
              <p>
                {t(
                  "serverFilesDrawer.connectWithAnSFTPClientUsingTheAddressAndPort",
                )}
              </p>
              <DetailsList>
                <DetailItem label={t("serverFilesDrawer.username")}>
                  <code>{server.id}</code>
                </DetailItem>
                <DetailItem label={t("serverFilesDrawer.password")}>
                  {t("serverFilesDrawer.yourPersonalAPIToken")}
                </DetailItem>
              </DetailsList>
              <p>
                <Trans
                  i18nKey="serverFilesDrawer.createATokenFromInYourAccountMenuWithAnd"
                  t={t}
                  components={{
                    strong: <strong>{t("serverFilesDrawer.apiKeys")}</strong>,
                    code: <code>{Permission.ServerView}</code>,
                    code2: <code>{Permission.ServerFilesRead}</code>,
                  }}
                />
              </p>
            </div>
          </details>
        )
      }
      actions={
        !editor && (
          <div className="file-toolbar">
            <Button
              type="button"
              variant="primary"
              disabled={busy || loading || !canWrite || maxUploadBytes === null}
              onClick={() => uploadInput.current?.click()}
            >
              <NavigationIcon name="upload" />
              {t("serverFilesDrawer.uploadFiles")}
            </Button>
            <Button
              disabled={busy || loading || !canWrite}
              onClick={() => {
                setCreating("file");
              }}
            >
              <NavigationIcon name="file-plus" />
              {t("serverFilesDrawer.createFile")}
            </Button>
            <Button
              disabled={busy || loading || !canWrite}
              onClick={() => {
                setCreating("folder");
              }}
            >
              <NavigationIcon name="folder-plus" />
              {t("serverFilesDrawer.createFolder")}
            </Button>
            <input
              ref={uploadInput}
              hidden
              type="file"
              multiple
              disabled={busy || loading || !canWrite || maxUploadBytes === null}
              aria-label={t("serverFilesDrawer.uploadFiles")}
              onChange={(event) => {
                const files = Array.from(event.target.files ?? []);
                event.target.value = "";
                if (files.length)
                  void perform(async () => {
                    for (const file of files) {
                      if (!alive.current) break;
                      await upload(file);
                    }
                  });
              }}
            />
          </div>
        )
      }
      busy={busy}
      onClose={close}
      footerLeft={
        editor ? (
          <Button
            disabled={busy}
            onClick={() => {
              if (
                editor.text !== editor.initial &&
                !window.confirm(
                  i18n.t("serverFilesDrawer.discardUnsavedFileChanges"),
                )
              )
                return;
              setEditor(null);
              void perform(() => load(directory));
            }}
          >
            {t("serverFilesDrawer.backToFiles")}
          </Button>
        ) : null
      }
      footerRight={
        editor ? (
          <Button
            variant="primary"
            disabled={busy || !canWrite}
            onClick={() => {
              void perform(save);
            }}
            loading={busy}
            loadingLabel={t("serverFilesDrawer.saving")}
          >
            {t("serverFilesDrawer.saveFile")}
          </Button>
        ) : null
      }
    >
      <div className="drawer-content">
        {bulkAction && (
          <InputDialog
            title={t(
              bulkAction === "compress"
                ? "serverFilesDrawer.archiveSelected"
                : "serverFilesDrawer.moveSelected",
            )}
            label={t(
              bulkAction === "compress"
                ? "serverFilesDrawer.archiveName"
                : "serverFilesDrawer.destinationFolder",
            )}
            description={t(
              bulkAction === "compress"
                ? "serverFilesDrawer.archiveDescription"
                : "serverFilesDrawer.moveDescription",
              { count: selectedEntries.length },
            )}
            submitLabel={t(
              bulkAction === "compress"
                ? "serverFilesDrawer.archiveSelected"
                : "serverFilesDrawer.move",
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
                ? t("serverFilesDrawer.rename2")
                : fileAction.kind === "compress"
                  ? t("serverFilesDrawer.compress2")
                  : t("serverFilesDrawer.extractArchive")
            }
            label={
              fileAction.kind === "rename"
                ? t("serverFilesDrawer.newName")
                : fileAction.kind === "compress"
                  ? t("serverFilesDrawer.archiveName")
                  : t("serverFilesDrawer.newFolderName")
            }
            description={t("serverFilesDrawer.in", {
              fileActionEntryName: fileAction.entry.name,
              directoryTheRoot:
                directory === "/"
                  ? t("serverFilesDrawer.theRootFolder")
                  : directory,
            })}
            submitLabel={
              fileAction.kind === "rename"
                ? t("serverFilesDrawer.rename2")
                : fileAction.kind === "compress"
                  ? t("serverFilesDrawer.compress2")
                  : t("serverFilesDrawer.extract")
            }
            initialValue={
              fileAction.kind === "rename"
                ? fileAction.entry.name
                : fileAction.kind === "compress"
                  ? `${fileAction.entry.name}.tar.gz`
                  : fileAction.entry.name.replace(/\.(tar\.gz|tgz|zip)$/i, "")
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
            title={t("serverFilesDrawer.permissions")}
            label={t("serverFilesDrawer.octalPermissions")}
            description={t("serverFilesDrawer.permissionsHelp", {
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
            submitLabel={t("serverFilesDrawer.savePermissions")}
            onClose={() => setPermissionsEntries(null)}
            onSubmit={async (value) => {
              if (!/^[0-7]{3}$/.test(value))
                throw new Error(t("serverFilesDrawer.invalidPermissions"));
              for (const entry of permissionsEntries)
                await manageFiles(server.id, {
                  operation: "chmod",
                  path: join(directory, entry.name),
                  mode: Number.parseInt(value, 8),
                });
              await load(directory);
            }}
          />
        )}
        {deleting && (
          <ConfirmDialog
            title={t("serverFilesDrawer.deleteSelected")}
            description={t("serverFilesDrawer.deleteSelectedDescription", {
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
                ? t("serverFilesDrawer.newFolder")
                : t("serverFilesDrawer.newFile")
            }
            label={t("common.name")}
            description={t("serverFilesDrawer.createIn", {
              directoryTheRoot:
                directory === "/"
                  ? t("serverFilesDrawer.theRootFolder")
                  : directory,
            })}
            submitLabel={
              creating === "folder"
                ? t("serverFilesDrawer.createFolder")
                : t("serverFilesDrawer.createFile")
            }
            onClose={() => setCreating(null)}
            onSubmit={(name) => create(creating === "folder", name)}
          />
        )}
        {progress && (
          <p role="status">
            {progress}{" "}
            <Button onClick={() => transfer.current?.abort()}>
              {t("serverFilesDrawer.cancelUpload")}
            </Button>
          </p>
        )}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {!canWrite && (
          <PrerequisiteNote>
            {t(
              hasWritePermission
                ? "serverFilesDrawer.fileModificationsAreCurrentlyUnavailableWaitForTheCurrentOperation"
                : "serverFilesDrawer.readOnlyAccess",
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
                {t("serverFilesDrawer.unsavedChanges")}
              </p>
            )}
          </>
        ) : (
          <>
            <nav
              className="file-breadcrumbs"
              aria-label={t("serverFilesDrawer.fileDirectory")}
            >
              <button
                className="file-breadcrumb"
                aria-label={t("serverFilesDrawer.root")}
                title={t("serverFilesDrawer.root")}
                disabled={busy}
                aria-current={directory === "/" ? "location" : undefined}
                onClick={() => {
                  void perform(() => load("/"));
                }}
              >
                <NavigationIcon name="folder" />
                {t("serverFilesDrawer.root")}
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
              className="file-list"
              empty={loading || entries.length === 0}
              loading={loading}
              resource={t("serverFilesDrawer.files2")}
              emptyMessage={t("serverFilesDrawer.thisFolderIsEmpty")}
            >
              <thead>
                <tr>
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
                        {t("serverFilesDrawer.selectAll")}
                      </span>
                    </CheckboxField>
                  </th>
                  <th className="file-icon-cell" />
                  <th className="file-name-column">{t("common.name")}</th>
                  <th className="table-fit">
                    {t("serverFilesDrawer.fileType")}
                  </th>
                  <th className="file-size">{t("common.size")}</th>
                  <th className="table-fit">
                    {t("serverFilesDrawer.modified")}
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
                    <td className="table-fit">
                      <CheckboxField
                        checked={selected.includes(entry.name)}
                        disabled={busy || entry.kind === "unsupported"}
                        onChange={(event) =>
                          setSelected((current) =>
                            event.target.checked
                              ? [...current, entry.name]
                              : current.filter((name) => name !== entry.name),
                          )
                        }
                      >
                        <span className="visually-hidden">
                          {t("serverFilesDrawer.selectEntry", {
                            name: entry.name,
                          })}
                        </span>
                      </CheckboxField>
                    </td>
                    <td className="file-icon-cell">
                      <span
                        className={`file-entry-icon${entry.kind === "directory" ? " file-entry-folder" : ""}`}
                      >
                        <NavigationIcon
                          name={
                            entry.kind === "directory"
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
                        {entry.kind === "unsupported" && (
                          <span className="file-entry-meta">
                            {t("serverFilesDrawer.symbolicLinkOrSpecialFile")}
                          </span>
                        )}
                      </button>
                    </td>
                    <td className="table-fit file-entry-meta">
                      {entry.kind === "directory"
                        ? t("serverFilesDrawer.folderType")
                        : entry.kind === "unsupported"
                          ? t("serverFilesDrawer.specialType")
                          : fileExtension(entry.name).toUpperCase() ||
                            t("serverFilesDrawer.fileTypeFallback")}
                    </td>
                    <td className="file-size file-entry-meta">
                      {entry.kind === "file" ? bytes(entry.size) : EMPTY_VALUE}
                    </td>
                    <td className="table-fit file-entry-meta">
                      <time dateTime={entry.modified_at}>
                        {new Date(entry.modified_at).toLocaleString()}
                      </time>
                    </td>
                    <td className="table-actions">
                      <ActionGroup>
                        {entry.kind === "file" &&
                          /\.(zip|tar\.gz|tgz)$/i.test(entry.name) && (
                            <Button
                              iconOnly
                              size="small"
                              title={i18n.t("serverFilesDrawer.extract")}
                              aria-label={i18n.t("serverFilesDrawer.extract2", {
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
                        {entry.kind === "file" && (
                          <Button
                            iconOnly
                            size="small"
                            title={i18n.t("serverFilesDrawer.download")}
                            disabled={busy || entry.size > fileTransferLimit}
                            onClick={() => {
                              void perform(() => download(entry));
                            }}
                            aria-label={i18n.t("serverFilesDrawer.download2", {
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
            <div
              className="file-selection-bar"
              role="group"
              aria-label={t("common.actions")}
            >
              <span className="file-selection-count">
                {t("serverFilesDrawer.selectedCount", {
                  count: selectedEntries.length,
                })}
              </span>
              <ActionGroup className="file-bulk-actions">
                <Button
                  disabled={busy || !canWrite || selectedEntries.length !== 1}
                  onClick={() => {
                    const entry = selectedEntries[0];
                    if (entry) setFileAction({ kind: "rename", entry });
                  }}
                >
                  <NavigationIcon name="rename" />
                  {t("serverFilesDrawer.rename2")}
                </Button>
                <Button
                  disabled={busy || !canWrite || !selectedEntries.length}
                  onClick={() => setPermissionsEntries(selectedEntries)}
                >
                  <NavigationIcon name="permissions" />
                  {t("serverFilesDrawer.permissions")}
                </Button>
                <Button
                  disabled={busy || !canWrite || !selectedEntries.length}
                  onClick={() => setBulkAction("move")}
                >
                  <NavigationIcon name="move" />
                  {t("serverFilesDrawer.move")}
                </Button>
                <Button
                  disabled={busy || !canWrite || !selectedEntries.length}
                  onClick={() => setBulkAction("compress")}
                >
                  <NavigationIcon name="compress" />
                  {t("serverFilesDrawer.archiveSelected")}
                </Button>
                <Button
                  className="file-bulk-delete"
                  disabled={busy || !canDelete || !selectedEntries.length}
                  onClick={() => {
                    setDeleteError(null);
                    setDeleting(selectedEntries);
                  }}
                >
                  <NavigationIcon name="delete" />
                  {t("common.delete")}
                </Button>
              </ActionGroup>
            </div>
          </>
        )}
      </div>
    </Drawer>
  );
}
