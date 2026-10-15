import { useEffect, useRef, useState } from "react";
import { listBackups } from "../api/backups";
import type { FileEntry } from "../api/files";
import { serverAction, serverStatus, type RuntimeStatus } from "../api/runtime";
import type { Server } from "../api/servers";
import { Drawer } from "../components/layout/Drawer";
import "../components/servers/files.css";

export function ServerBackupsDialog({
  server,
  onClose,
}: {
  server: Server;
  onClose: () => void;
}) {
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [status, setStatus] = useState<RuntimeStatus>();
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const pending = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function poll() {
      try {
        const signal = AbortSignal.any([
          controller.signal,
          AbortSignal.timeout(12000),
        ]);
        const next = await serverStatus(server.id, signal);
        const backups = next.busy ? null : await listBackups(server.id, signal);
        if (!controller.signal.aborted && !pending.current) {
          setStatus(next);
          setLoadError(null);
          if (backups) setEntries(backups);
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setLoadError(
            cause instanceof Error ? cause.message : "Unable to load backups.",
          );
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => {
          void poll();
        }, 2000);
    }
    void poll();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [server.id, refresh]);
  const canChange =
    status?.state === "stopped" && !status.busy && !busy && loadError === null;
  async function submit(action: "backup" | "restore", name?: string) {
    if (pending.current || !canChange) return;
    if (
      action === "restore" &&
      !window.confirm(
        `Restore this backup of "${server.name}"? This replaces its current game files.`,
      )
    )
      return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      const next = await serverAction(server.id, action, name);
      if (mounted.current) {
        setStatus(next);
        setRefresh((value) => value + 1);
      }
    } catch (cause) {
      if (mounted.current)
        setError(
          cause instanceof Error
            ? cause.message
            : "Unable to submit operation.",
        );
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <Drawer
      title={`${server.name} backups`}
      description="Stop the server to create or restore a backup. Backups are stored on its Spark."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <button className="button" disabled={busy} onClick={onClose}>
          Close
        </button>
      }
      footerRight={
        <button
          className="button button-primary"
          disabled={!canChange}
          onClick={() => {
            void submit("backup");
          }}
        >
          Create backup
        </button>
      }
    >
      <div className="drawer-content">
        {loadError && (
          <p className="error-message" role="alert">
            {loadError}
          </p>
        )}
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        {status?.error && (
          <p className="error-message" role="alert">
            {status.error}
          </p>
        )}
        <p role="status">
          {status?.busy
            ? status.progress
            : status
              ? `${entries.length} backups`
              : "Loading…"}
        </p>
        {!status?.busy &&
          !status?.error &&
          (status?.operation === "backup" ||
            status?.operation === "restore") && (
            <p role="status">
              {status.operation === "backup"
                ? "Backup created."
                : "Backup restored. The server is stopped."}
            </p>
          )}
        {entries.map((entry) => (
          <div className="file-row" key={entry.name}>
            <div className="file-name">
              <p>{new Date(entry.modified_at).toLocaleString()}</p>
              <p>{entry.name}</p>
              <p>{(entry.size / (1024 * 1024)).toFixed(2)} MiB</p>
            </div>
            <div className="row-actions">
              {!busy && !status?.busy && (
                <a
                  className="button"
                  href={`/api/v1/servers/${encodeURIComponent(server.id)}/backups/${encodeURIComponent(entry.name)}`}
                  download
                >
                  Download
                </a>
              )}
              <button
                className="button"
                disabled={!canChange}
                onClick={() => {
                  void submit("restore", entry.name);
                }}
              >
                Restore
              </button>
            </div>
          </div>
        ))}
        <p>
          Backups exclude links and special files and are limited to 100 GiB and
          100,000 entries. Deleting the server also deletes its backups.
        </p>
      </div>
    </Drawer>
  );
}
