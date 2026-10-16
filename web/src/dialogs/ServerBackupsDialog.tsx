import { useEffect, useRef, useState } from "react";
import {
  deleteBackup,
  listBackups,
  saveBackupPolicy,
  type Backup,
  type BackupPolicy,
} from "../api/backups";
import { serverAction, serverStatus, type RuntimeStatus } from "../api/runtime";
import type { Server } from "../api/servers";
import { Drawer } from "../components/layout/Drawer";
import "../components/servers/files.css";

export function ServerBackupsDialog({
  server,
  onClose,
  embedded = false,
}: {
  server: Server;
  onClose: () => void;
  embedded?: boolean;
}) {
  const [entries, setEntries] = useState<Backup[]>([]);
  const [policy, setPolicy] = useState<BackupPolicy | null>(null);
  const [s3Available, setS3Available] = useState(false);
  const [runtimeError, setRuntimeError] = useState<string | null>(null);
  const policyDirty = useRef(false);
  const [policyEdited, setPolicyEdited] = useState(false);
  const canConfigure = server.permissions.includes("server.configure");
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
        const [observed, listed] = await Promise.allSettled([
          serverStatus(server.id, signal),
          listBackups(server.id, signal),
        ]);
        if (listed.status === "rejected") throw listed.reason;
        const backups = listed.value;
        if (!controller.signal.aborted && !pending.current) {
          setStatus(
            observed.status === "fulfilled" ? observed.value : undefined,
          );
          setRuntimeError(
            observed.status === "fulfilled"
              ? null
              : "Spark is unavailable. Off-host backups can still be downloaded.",
          );
          setLoadError(null);
          setEntries(backups.entries);
          setS3Available(backups.s3_available);
          if (!policyDirty.current) setPolicy(backups.policy);
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
  function changePolicy(key: keyof BackupPolicy, value: string | number) {
    if (!policy) return;
    policyDirty.current = true;
    setPolicyEdited(true);
    setPolicy({ ...policy, [key]: value });
  }
  async function savePolicy() {
    if (!policy || pending.current) return;
    if (
      !window.confirm(
        "Apply this backup policy? Retention runs in the background and permanently deletes expired copies independently from local and S3 storage. Zero disables that limit.",
      )
    )
      return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      const saved = await saveBackupPolicy(server.id, policy);
      if (mounted.current) {
        setPolicy(saved);
        policyDirty.current = false;
        setPolicyEdited(false);
        setRefresh((value) => value + 1);
      }
    } catch (cause) {
      if (mounted.current)
        setError(
          cause instanceof Error
            ? cause.message
            : "Unable to save retention policy.",
        );
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  async function remove(entry: Backup, location: "local" | "s3") {
    if (
      pending.current ||
      !window.confirm(
        `Permanently delete the ${location === "local" ? "local" : "S3"} copy of ${entry.name}? Other copies are kept. Deleting an S3 copy disables background resync for that archive.`,
      )
    )
      return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await deleteBackup(server.id, entry.name, location);
      if (mounted.current) setRefresh((value) => value + 1);
    } catch (cause) {
      if (mounted.current)
        setError(
          cause instanceof Error ? cause.message : "Unable to delete backup.",
        );
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
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
      embedded={embedded}
      title={`${server.name} backups`}
      description="Stop the server to create or restore a backup. Completed archives sync to S3 in the background when enabled."
      busy={busy}
      onClose={onClose}
      footerLeft={
        embedded ? null : (
          <button className="button" disabled={busy} onClick={onClose}>
            Close
          </button>
        )
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
        {runtimeError && <p role="status">{runtimeError}</p>}
        {canConfigure && policy && (
          <details>
            <summary>Storage and retention</summary>
            <fieldset disabled={busy}>
              <label htmlFor="backup-storage">Background S3 sync</label>
              <select
                id="backup-storage"
                value={policy.storage}
                onChange={(event) =>
                  changePolicy("storage", event.target.value)
                }
              >
                <option value="local">Disabled</option>
                <option value="s3" disabled={!s3Available}>
                  Enabled
                </option>
              </select>
              {!s3Available && (
                <p className="field-help">
                  An administrator must configure S3 on Campfire before enabling
                  sync.
                </p>
              )}
              {(
                [
                  ["local_keep_last", "Local copies to keep", 1000],
                  ["local_max_age_days", "Local maximum age (days)", 36500],
                  ["remote_keep_last", "S3 copies to keep", 1000],
                  ["remote_max_age_days", "S3 maximum age (days)", 36500],
                ] as const
              ).map(([key, label, max]) => (
                <div key={key}>
                  <label htmlFor={`backup-${key}`}>{label}</label>
                  <input
                    id={`backup-${key}`}
                    type="number"
                    min={0}
                    max={max}
                    step={1}
                    value={policy[key]}
                    onChange={(event) =>
                      changePolicy(key, Number(event.target.value))
                    }
                  />
                </div>
              ))}
              <p className="field-help">
                Zero disables a limit. Local and S3 copies expire independently.
                A local copy awaiting a required S3 upload is retained until
                sync succeeds.
              </p>
              <button
                className="button"
                disabled={busy || !policyEdited}
                onClick={() => {
                  void savePolicy();
                }}
              >
                Save backup policy
              </button>
            </fieldset>
          </details>
        )}
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
              <p>
                {[entry.local ? "Local" : null, entry.remote ? "S3" : null]
                  .filter(Boolean)
                  .join(" · ") || "No available copy"}{" "}
                · {entry.state}
              </p>
              {entry.error && <p role="alert">{entry.error}</p>}
            </div>
            <div className="row-actions">
              {!busy &&
                entry.state !== "deleting" &&
                (entry.remote || (entry.local && status && !status.busy)) && (
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
                disabled={
                  !canChange ||
                  entry.state === "deleting" ||
                  (!entry.local && !entry.remote)
                }
                onClick={() => {
                  void submit("restore", entry.name);
                }}
              >
                Restore
              </button>
              {canConfigure && entry.local && (
                <button
                  className="button button-danger"
                  disabled={
                    busy || status?.busy === true || entry.state === "deleting"
                  }
                  onClick={() => {
                    void remove(entry, "local");
                  }}
                >
                  Delete local
                </button>
              )}
              {canConfigure && entry.remote && (
                <button
                  className="button button-danger"
                  disabled={
                    busy || status?.busy === true || entry.state === "deleting"
                  }
                  onClick={() => {
                    void remove(entry, "s3");
                  }}
                >
                  Delete S3
                </button>
              )}
            </div>
          </div>
        ))}
        <p>
          Backups exclude links and special files and are limited to 100 GiB and
          100,000 entries. Manual and scheduled backups use this policy.
          Deleting the server also deletes its local and S3 backups.
        </p>
      </div>
    </Drawer>
  );
}
