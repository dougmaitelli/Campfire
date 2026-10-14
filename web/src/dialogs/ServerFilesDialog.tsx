import { useCallback, useEffect, useRef, useState } from "react";
import type { Server } from "../api/servers";
import { decodeBytes, encodeBytes, fileLimit, manageFiles } from "../api/files";
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
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(true);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
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
    void load(".", controller.signal)
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
  }, [load]);
  async function perform(work: () => Promise<void>) {
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
  async function download(entry: FileEntry) {
    const result = await manageFiles(server.id, {
      operation: "read",
      path: join(directory, entry.name),
    });
    const url = URL.createObjectURL(
      new Blob([decodeBytes(result.data ?? "")], {
        type: "application/octet-stream",
      }),
    );
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = entry.name;
    anchor.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
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
    if (file.size > fileLimit)
      throw new Error("Transfers are limited to 1 MiB per file.");
    const name = entryName(file.name);
    if (name === null) return;
    await manageFiles(server.id, {
      operation: "write",
      path: join(directory, name),
      data: encodeBytes(new Uint8Array(await file.arrayBuffer())),
      revision: "absent",
    });
    await load(directory);
  }
  return (
    <Drawer
      title={`${server.name} files`}
      description="Browse game files. Transfers are limited to 1 MiB per file."
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
                  disabled={busy || loading || !canWrite}
                  aria-label="Upload file"
                  onChange={(event) => {
                    const file = event.target.files?.item(0);
                    event.target.value = "";
                    if (file) void perform(() => upload(file));
                  }}
                />
              </label>
            </div>
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
                          disabled={busy || entry.size > fileLimit}
                          onClick={() => {
                            void perform(() => download(entry));
                          }}
                          aria-label={`Download ${entry.name}`}
                        >
                          Download
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
