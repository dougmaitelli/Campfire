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
      "Enter a file or folder name without slashes or control characters.",
    );
  return name;
}

export function ServerFilesDialog({
  server,
  canWrite,
  onClose,
}: {
  server: Server;
  canWrite: boolean;
  onClose: () => void;
}) {
  const [directory, setDirectory] = useState(".");
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [editor, setEditor] = useState<Editor | null>(null);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [maxUploadBytes, setMaxUploadBytes] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(true);
  const alive = useRef(true);
  const transfer = useRef<AbortController | null>(null);
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
            cause instanceof Error ? cause.message : "Unable to load files.",
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
          cause instanceof Error ? cause.message : "File operation failed.",
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
      !window.confirm("Discard unsaved file changes?")
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
        "Text editing is limited to 1 MiB. Download this file to edit it locally.",
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
      throw new Error("This file is binary. Use Download instead of editing.");
    }
    if (!result.revision) throw new Error("File revision is missing.");
    setEditor({
      path: result.path,
      text,
      initial: text,
      revision: result.revision,
    });
  }
  function download(entry: FileEntry) {
    if (entry.size > fileTransferLimit)
      throw new Error("Downloads are limited to 100 GiB.");
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
    if (!result.revision) throw new Error("File revision is missing.");
    setEditor({ ...editor, initial: editor.text, revision: result.revision });
  }
  async function create(folder: boolean) {
    const name = entryName(
      window.prompt(folder ? "New folder name" : "New file name"),
    );
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
  async function rename(entry: FileEntry) {
    const name = entryName(window.prompt("New name", entry.name));
    if (name === null || name === entry.name) return;
    await manageFiles(server.id, {
      operation: "rename",
      path: join(directory, entry.name),
      destination: join(directory, name),
    });
    await load(directory);
  }
  async function remove(entry: FileEntry) {
    if (
      !window.confirm(
        `Permanently delete "${entry.name}"? Folders must be empty.`,
      )
    )
      return;
    await manageFiles(server.id, {
      operation: "delete",
      path: join(directory, entry.name),
    });
    await load(directory);
  }
  async function upload(file: File) {
    if (maxUploadBytes === null)
      throw new Error(
        "Upload limit is unavailable. Reopen Files to try again.",
      );
    if (file.size > maxUploadBytes)
      throw new Error(
        `Uploads are limited to ${(maxUploadBytes / 1024 ** 2).toLocaleString()} MiB per file.`,
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
      if (!uploadID) throw new Error("Upload session is missing.");
      for (let offset = 0; offset < file.size; offset += fileLimit) {
        if (alive.current)
          setProgress(
            `Uploading ${file.name}: ${Math.floor((offset / file.size) * 100)}%`,
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
  async function archive(entry: FileEntry, extract: boolean) {
    const destination = entryName(
      window.prompt(
        extract ? "Extract into a new folder" : "New .tar.gz archive name",
        extract
          ? entry.name.replace(/\.(tar\.gz|tgz|zip)$/i, "")
          : `${entry.name}.tar.gz`,
      ),
    );
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
      title={`${server.name} files`}
      description={`Browse game files. ${maxUploadBytes === null ? "Loading upload limit." : `Uploads support up to ${(maxUploadBytes / 1024 ** 2).toLocaleString()} MiB per file.`} Text editing supports 1 MiB.`}
      busy={busy}
      onClose={close}
      footerLeft={
        <button
          className="button"
          disabled={busy}
          onClick={() => {
            if (editor) {
              if (
                editor.text !== editor.initial &&
                !window.confirm("Discard unsaved file changes?")
              )
                return;
              setEditor(null);
              void perform(() => load(directory));
            } else close();
          }}
        >
          {editor ? "Back to files" : "Close"}
        </button>
      }
      footerRight={
        editor ? (
          <button
            className="button button-primary"
            disabled={busy || !canWrite}
            onClick={() => {
              void perform(save);
            }}
          >
            {busy ? "Saving…" : "Save file"}
          </button>
        ) : (
          <button
            className="button"
            disabled={busy || loading}
            onClick={() => {
              void perform(() => load(directory));
            }}
          >
            Refresh
          </button>
        )
      }
    >
      <div className="drawer-content">
        {progress && (
          <p role="status">
            {progress}{" "}
            <button
              className="button"
              onClick={() => transfer.current?.abort()}
            >
              Cancel upload
            </button>
          </p>
        )}
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        {!canWrite && (
          <p className="prerequisite-note">
            Stop the server to modify its files. Browsing and downloads remain
            available.
          </p>
        )}
        {editor ? (
          <>
            <label htmlFor="server-file-content">{editor.path}</label>
            <textarea
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
            {editor.text !== editor.initial && (
              <p className="list-summary" role="status">
                Unsaved changes
              </p>
            )}
          </>
        ) : (
          <>
            <nav className="file-breadcrumbs" aria-label="File directory">
              <button
                className="button"
                disabled={busy}
                onClick={() => {
                  void perform(() => load("."));
                }}
              >
                Game files
              </button>
              {directory !== "." &&
                directory.split("/").map((part, index, parts) => (
                  <button
                    className="button"
                    disabled={busy}
                    key={parts.slice(0, index + 1).join("/")}
                    onClick={() => {
                      void perform(() =>
                        load(parts.slice(0, index + 1).join("/")),
                      );
                    }}
                  >
                    {part}
                  </button>
                ))}
            </nav>
            <div className="file-toolbar">
              <button
                className="button"
                disabled={busy || loading || !canWrite}
                onClick={() => {
                  void perform(() => create(false));
                }}
              >
                New file
              </button>
              <button
                className="button"
                disabled={busy || loading || !canWrite}
                onClick={() => {
                  void perform(() => create(true));
                }}
              >
                New folder
              </button>
              <label className="button file-upload">
                Upload file
                <input
                  type="file"
                  disabled={
                    busy || loading || !canWrite || maxUploadBytes === null
                  }
                  aria-label="Upload file"
                  onChange={(event) => {
                    const file = event.target.files?.item(0);
                    event.target.value = "";
                    if (file) void perform(() => upload(file));
                  }}
                />
              </label>
            </div>
            <details>
              <summary>SFTP access</summary>
              <p>
                When your administrator enables SFTP, connect to Campfire’s SFTP
                address using <code>{server.id}</code> as the username and a
                personal API token as the password. The token needs server.view
                and server.operate. Create tokens from API tokens in the
                sidebar. Stop the server before uploading or modifying files.
              </p>
            </details>
            <div className="file-list">
              {loading ? (
                <p className="list-summary" role="status">
                  Loading files…
                </p>
              ) : entries.length === 0 ? (
                <p className="list-summary">This folder is empty.</p>
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
                      {entry.name}
                      <span className="row-description">
                        {entry.kind === "directory"
                          ? "Folder"
                          : entry.kind === "unsupported"
                            ? "Symbolic link or special file"
                            : `${entry.size.toLocaleString()} bytes`}{" "}
                        · {new Date(entry.modified_at).toLocaleString()}
                      </span>
                    </button>
                    <div className="row-actions">
                      {entry.kind === "file" && (
                        <button
                          className="button"
                          disabled={busy || entry.size > fileTransferLimit}
                          onClick={() => {
                            void perform(() => download(entry));
                          }}
                          aria-label={`Download ${entry.name}`}
                        >
                          Download
                        </button>
                      )}
                      {entry.kind !== "unsupported" && (
                        <button
                          className="button"
                          disabled={busy || !canWrite}
                          onClick={() => {
                            void perform(() => archive(entry, false));
                          }}
                        >
                          Compress
                        </button>
                      )}
                      {entry.kind === "file" &&
                        /\.(zip|tar\.gz|tgz)$/i.test(entry.name) && (
                          <button
                            className="button"
                            disabled={busy || !canWrite}
                            onClick={() => {
                              void perform(() => archive(entry, true));
                            }}
                          >
                            Extract
                          </button>
                        )}
                      <button
                        className="button"
                        disabled={
                          busy || !canWrite || entry.kind === "unsupported"
                        }
                        onClick={() => {
                          void perform(() => rename(entry));
                        }}
                        aria-label={`Rename ${entry.name}`}
                      >
                        Rename
                      </button>
                      <button
                        className="button"
                        disabled={
                          busy || !canWrite || entry.kind === "unsupported"
                        }
                        onClick={() => {
                          void perform(() => remove(entry));
                        }}
                        aria-label={`Delete ${entry.name}`}
                      >
                        Delete
                      </button>
                    </div>
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
