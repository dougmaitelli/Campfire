import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { PrerequisiteNote } from "../components/status/PrerequisiteNote";
import { ActionGroup } from "../components/layout/ActionGroup";
import { DetailsList, DetailItem } from "../components/layout/DetailsList";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { ContentState } from "../components/layout/ContentState";
import { InlineMessage } from "../components/status/InlineMessage";
import { Textarea } from "../components/forms/Textarea";
import { useCallback, useEffect, useRef, useState } from "react";
import type { Server } from "../api/servers";
import {
  decodeBytes,
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
import "../components/servers/files.css";

interface Editor {
  path: string;
  text: string;
  initial: string;
  revision: string;
}

function join(directory: string, name: string) {
  return directory === "." ? name : `${directory}/${name}`;
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
      i18n.t("serverFilesDialog.enterAFileOrFolderNameWithoutSlashesOrControl"),
    );
  return name;
}

export function ServerFilesDialog({
  server,
  canWrite,
  onClose,
  embedded = false,
}: {
  server: Server;
  canWrite: boolean;
  onClose: () => void;
  embedded?: boolean;
}) {
  const { t } = useTranslation();
  const [directory, setDirectory] = useState(".");
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
  const [deleting, setDeleting] = useState<FileEntry | null>(null);
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
    async (path: string, signal?: AbortSignal) => {
      const result = await manageFiles(
        server.id,
        { operation: "list", path },
        signal,
      );
      if (!signal?.aborted && alive.current) {
        setEntries(result.entries ?? []);
        setDirectory(path);
      }
    },
    [server.id],
  );
  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([
      load(".", controller.signal),
      uploadLimit(
        server.id,
        AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
      ).then((limit) => {
        if (!controller.signal.aborted) setMaxUploadBytes(limit);
      }),
    ])
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverFilesDialog.unableToLoadFiles"),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          pending.current = false;
          setLoading(false);
        }
      });
    return () => controller.abort();
  }, [load, server.id]);
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
            : i18n.t("serverFilesDialog.fileOperationFailed"),
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
      !window.confirm(i18n.t("serverFilesDialog.discardUnsavedFileChanges"))
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
        i18n.t("serverFilesDialog.textEditingIsLimitedTo1MiBDownloadThisFile"),
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
        i18n.t("serverFilesDialog.thisFileIsBinaryUseDownloadInsteadOfEditing"),
      );
    }
    if (!result.revision)
      throw new Error(i18n.t("serverFilesDialog.fileRevisionIsMissing"));
    setEditor({
      path: result.path,
      text,
      initial: text,
      revision: result.revision,
    });
  }
  function download(entry: FileEntry) {
    if (entry.size > fileTransferLimit)
      throw new Error(i18n.t("serverFilesDialog.downloadsAreLimitedTo100GiB"));
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
      throw new Error(i18n.t("serverFilesDialog.fileRevisionIsMissing"));
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
  async function remove(entry: FileEntry) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setDeleteError(null);
    try {
      await manageFiles(server.id, {
        operation: "delete",
        path: join(directory, entry.name),
      });
      await load(directory);
      setDeleting(null);
    } catch (cause) {
      setDeleteError(
        cause instanceof Error
          ? cause.message
          : i18n.t("serverFilesDialog.unableToDeleteThisEntry"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  async function upload(file: File) {
    if (maxUploadBytes === null)
      throw new Error(
        i18n.t(
          "serverFilesDialog.uploadLimitIsUnavailableReopenFilesToTryAgain",
        ),
      );
    if (file.size > maxUploadBytes)
      throw new Error(
        i18n.t("serverFilesDialog.uploadsAreLimitedToMiBPerFile", {
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
        throw new Error(i18n.t("serverFilesDialog.uploadSessionIsMissing"));
      for (let offset = 0; offset < file.size; offset += fileLimit) {
        if (alive.current)
          setProgress(
            i18n.t("serverFilesDialog.uploading", {
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
  return (
    <Drawer
      embedded={embedded}
      unsavedChanges={editor !== null && editor.text !== editor.initial}
      title={t("serverFilesDialog.files", { serverName: server.name })}
      description={t(
        "serverFilesDialog.browseGameFilesTextEditingSupports1MiB",
        {
          maxUploadBytesNullLoading:
            maxUploadBytes === null
              ? t("serverFilesDialog.loadingUploadLimit")
              : t("serverFilesDialog.uploadsSupportUpToMiBPerFile", {
                  toLocaleString: (maxUploadBytes / 1024 ** 2).toLocaleString(),
                }),
        },
      )}
      busy={busy}
      onClose={close}
      footerLeft={
        embedded && !editor ? null : (
          <Button
            disabled={busy}
            onClick={() => {
              if (editor) {
                if (
                  editor.text !== editor.initial &&
                  !window.confirm(
                    i18n.t("serverFilesDialog.discardUnsavedFileChanges"),
                  )
                )
                  return;
                setEditor(null);
                void perform(() => load(directory));
              } else close();
            }}
          >
            {editor ? t("serverFilesDialog.backToFiles") : t("common.close")}
          </Button>
        )
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
            loadingLabel={t("serverFilesDialog.saving")}
          >
            {t("serverFilesDialog.saveFile")}
          </Button>
        ) : null
      }
    >
      <div className="drawer-content">
        {fileAction && (
          <InputDialog
            title={
              fileAction.kind === "rename"
                ? t("serverFilesDialog.rename2")
                : fileAction.kind === "compress"
                  ? t("serverFilesDialog.compress2")
                  : t("serverFilesDialog.extractArchive")
            }
            label={
              fileAction.kind === "rename"
                ? t("serverFilesDialog.newName")
                : fileAction.kind === "compress"
                  ? t("serverFilesDialog.archiveName")
                  : t("serverFilesDialog.newFolderName")
            }
            description={t("serverFilesDialog.in", {
              fileActionEntryName: fileAction.entry.name,
              directoryTheRoot:
                directory === "."
                  ? t("serverFilesDialog.theRootFolder")
                  : directory,
            })}
            submitLabel={
              fileAction.kind === "rename"
                ? t("serverFilesDialog.rename2")
                : fileAction.kind === "compress"
                  ? t("serverFilesDialog.compress2")
                  : t("serverFilesDialog.extract")
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
        {deleting && (
          <ConfirmDialog
            title={
              deleting.kind === "directory"
                ? t("serverFilesDialog.deleteFolder")
                : t("serverFilesDialog.deleteFile")
            }
            description={t(
              "serverFilesDialog.permanentlyDeleteThisCannotBeUndone",
              {
                deletingName: deleting.name,
                deletingKindDirectory:
                  deleting.kind === "directory"
                    ? t("serverFilesDialog.theFolderMustBeEmpty")
                    : "",
              },
            )}
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
                ? t("serverFilesDialog.newFolder")
                : t("serverFilesDialog.newFile")
            }
            label={t("common.name")}
            description={t("serverFilesDialog.createIn", {
              directoryTheRoot:
                directory === "."
                  ? t("serverFilesDialog.theRootFolder")
                  : directory,
            })}
            submitLabel={
              creating === "folder"
                ? t("serverFilesDialog.createFolder")
                : t("serverFilesDialog.createFile")
            }
            onClose={() => setCreating(null)}
            onSubmit={(name) => create(creating === "folder", name)}
          />
        )}
        {progress && (
          <p role="status">
            {progress}{" "}
            <Button onClick={() => transfer.current?.abort()}>
              {t("serverFilesDialog.cancelUpload")}
            </Button>
          </p>
        )}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {!canWrite && (
          <PrerequisiteNote>
            {t(
              "serverFilesDialog.fileModificationsAreCurrentlyUnavailableWaitForTheCurrentOperation",
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
                {t("serverFilesDialog.unsavedChanges")}
              </p>
            )}
          </>
        ) : (
          <>
            <details className="file-sftp">
              <summary>{t("serverFilesDialog.sftpAccess")}</summary>
              <div className="file-sftp-content">
                <p>
                  {t(
                    "serverFilesDialog.connectWithAnSFTPClientUsingTheAddressAndPort",
                  )}
                </p>
                <DetailsList>
                  <DetailItem label={t("serverFilesDialog.username")}>
                    <code>{server.id}</code>
                  </DetailItem>
                  <DetailItem label={t("serverFilesDialog.password")}>
                    {t("serverFilesDialog.yourPersonalAPIToken")}
                  </DetailItem>
                </DetailsList>
                <p>
                  <Trans
                    i18nKey="serverFilesDialog.createATokenFromInYourAccountMenuWithAnd"
                    t={t}
                    components={{
                      strong: <strong>{t("serverFilesDialog.apiKeys")}</strong>,
                      code: <code>{t("serverFilesDialog.serverView")}</code>,
                      code2: (
                        <code>{t("serverFilesDialog.serverOperate")}</code>
                      ),
                    }}
                  />
                </p>
              </div>
            </details>
            <div className="file-toolbar">
              <Button
                disabled={busy || loading || !canWrite}
                onClick={() => {
                  setCreating("file");
                }}
              >
                {t("serverFilesDialog.newFile")}
              </Button>
              <Button
                disabled={busy || loading || !canWrite}
                onClick={() => {
                  setCreating("folder");
                }}
              >
                {t("serverFilesDialog.newFolder")}
              </Button>
              <Button
                type="button"
                disabled={
                  busy || loading || !canWrite || maxUploadBytes === null
                }
                onClick={() => uploadInput.current?.click()}
              >
                {t("serverFilesDialog.uploadFile")}
              </Button>
              <Button
                className="file-refresh"
                disabled={busy || loading}
                onClick={() => {
                  void perform(() => load(directory));
                }}
              >
                {t("common.refresh")}
              </Button>
              <input
                ref={uploadInput}
                hidden
                type="file"
                disabled={
                  busy || loading || !canWrite || maxUploadBytes === null
                }
                aria-label={t("serverFilesDialog.uploadFile")}
                onChange={(event) => {
                  const file = event.target.files?.item(0);
                  event.target.value = "";
                  if (file) void perform(() => upload(file));
                }}
              />
            </div>
            <nav
              className="file-breadcrumbs"
              aria-label={t("serverFilesDialog.fileDirectory")}
            >
              <button
                className="file-breadcrumb"
                disabled={busy}
                aria-current={directory === "." ? "location" : undefined}
                onClick={() => {
                  void perform(() => load("."));
                }}
              >
                {t("serverFilesDialog.root")}
              </button>
              {directory !== "." &&
                directory.split("/").map((part, index, parts) => (
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
                          load(parts.slice(0, index + 1).join("/")),
                        );
                      }}
                    >
                      {part}
                    </button>
                  </span>
                ))}
            </nav>
            <div className="file-list">
              {directory !== "." && (
                <div className="file-row">
                  <button
                    type="button"
                    className="file-name"
                    aria-label={t("serverFilesDialog.goToParentFolder")}
                    disabled={busy || loading}
                    onClick={() => {
                      const parent = directory
                        .split("/")
                        .slice(0, -1)
                        .join("/");
                      void perform(() => load(parent || "."));
                    }}
                  >
                    <span className="file-entry-name">../</span>
                    <span className="file-entry-meta">
                      {t("serverFilesDialog.parentFolder")}
                    </span>
                  </button>
                </div>
              )}
              {loading ? (
                <ContentState
                  loading
                  resource={t("serverFilesDialog.files2")}
                  compact
                />
              ) : entries.length === 0 ? (
                <ContentState
                  resource={t("serverFilesDialog.files2")}
                  emptyMessage={t("serverFilesDialog.thisFolderIsEmpty")}
                  compact
                />
              ) : (
                entries.map((entry) => (
                  <div className="file-row" key={entry.name}>
                    <button
                      className="file-name"
                      disabled={busy || entry.kind === "unsupported"}
                      onClick={() => {
                        void perform(() => open(entry));
                      }}
                    >
                      <span className="file-entry-name">{entry.name}</span>
                      <span className="file-entry-meta">
                        {entry.kind === "directory"
                          ? i18n.t("serverFilesDialog.folder2")
                          : entry.kind === "unsupported"
                            ? i18n.t(
                                "serverFilesDialog.symbolicLinkOrSpecialFile",
                              )
                            : `${entry.size.toLocaleString()} bytes`}{" "}
                        · {new Date(entry.modified_at).toLocaleString()}
                      </span>
                    </button>
                    <ActionGroup>
                      {entry.kind === "file" && (
                        <Button
                          iconOnly
                          size="small"
                          title={i18n.t("serverFilesDialog.download")}
                          disabled={busy || entry.size > fileTransferLimit}
                          onClick={() => {
                            void perform(() => download(entry));
                          }}
                          aria-label={i18n.t("serverFilesDialog.download2", {
                            entryName: entry.name,
                          })}
                        >
                          <NavigationIcon name="download" />
                        </Button>
                      )}
                      {entry.kind !== "unsupported" && (
                        <Button
                          iconOnly
                          size="small"
                          title={i18n.t("serverFilesDialog.compress2")}
                          aria-label={i18n.t("serverFilesDialog.compress3", {
                            entryName: entry.name,
                          })}
                          disabled={busy || !canWrite}
                          onClick={() => {
                            setFileAction({ kind: "compress", entry });
                          }}
                        >
                          <NavigationIcon name="compress" />
                        </Button>
                      )}
                      {entry.kind === "file" &&
                        /\.(zip|tar\.gz|tgz)$/i.test(entry.name) && (
                          <Button
                            iconOnly
                            size="small"
                            title={i18n.t("serverFilesDialog.extract")}
                            aria-label={i18n.t("serverFilesDialog.extract2", {
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
                      <Button
                        iconOnly
                        size="small"
                        title={i18n.t("serverFilesDialog.rename2")}
                        disabled={
                          busy || !canWrite || entry.kind === "unsupported"
                        }
                        onClick={() => {
                          setFileAction({ kind: "rename", entry });
                        }}
                        aria-label={i18n.t("serverFilesDialog.rename3", {
                          entryName: entry.name,
                        })}
                      >
                        <NavigationIcon name="rename" />
                      </Button>
                      <Button
                        variant="danger"
                        iconOnly
                        size="small"
                        title={i18n.t("common.delete")}
                        disabled={
                          busy || !canWrite || entry.kind === "unsupported"
                        }
                        onClick={() => {
                          setDeleteError(null);
                          setDeleting(entry);
                        }}
                        aria-label={i18n.t("serverFilesDialog.delete", {
                          entryName: entry.name,
                        })}
                      >
                        <NavigationIcon name="delete" />
                      </Button>
                    </ActionGroup>
                  </div>
                ))
              )}
            </div>
          </>
        )}
      </div>
    </Drawer>
  );
}
