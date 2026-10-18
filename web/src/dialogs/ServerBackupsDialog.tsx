import { Button, ButtonLink } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { useEffect, useRef, useState } from "react";
import { deleteBackup, listBackups, type Backup } from "../api/backups";
import { serverAction, serverStatus, type RuntimeStatus } from "../api/runtime";
import type { Server } from "../api/servers";
import { Drawer } from "../components/layout/Drawer";
import { Table } from "../components/table/Table";
import { BackupState } from "../components/servers/BackupState";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { bytes } from "../api/monitoring";
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
  const [runtimeError, setRuntimeError] = useState<string | null>(null);
  const canConfigure = server.permissions.includes("server.configure");
  const [status, setStatus] = useState<RuntimeStatus>();
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
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
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setLoadError(
            cause instanceof Error ? cause.message : "Unable to load backups.",
          );
      } finally {
        if (!controller.signal.aborted) setLoading(false);
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
          <Button disabled={busy} onClick={onClose}>
            Close
          </Button>
        )
      }
      footerRight={
        <Button
          variant="primary"
          disabled={!canChange}
          onClick={() => {
            void submit("backup");
          }}
        >
          Create backup
        </Button>
      }
    >
      <div className="drawer-content">
        {runtimeError && (
          <InlineMessage tone="warning">{runtimeError}</InlineMessage>
        )}
        {loadError && entries.length > 0 && (
          <InlineMessage role="alert">{loadError}</InlineMessage>
        )}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {status?.error && (
          <InlineMessage role="alert">{status.error}</InlineMessage>
        )}
        {status?.busy && (
          <InlineMessage tone="info">{status.progress}</InlineMessage>
        )}
        {!status?.busy &&
          !status?.error &&
          (status?.operation === "backup" ||
            status?.operation === "restore") && (
            <InlineMessage tone="success">
              {status.operation === "backup"
                ? "Backup created."
                : "Backup restored. The server is stopped."}
            </InlineMessage>
          )}
        <Table
          empty={entries.length === 0}
          loading={loading}
          error={loadError}
          resource="backups"
          onRetry={() => {
            setLoading(true);
            setRefresh((value) => value + 1);
          }}
        >
          <caption className="visually-hidden">Server backups</caption>
          <thead>
            <tr>
              <th>Archive</th>
              <th>Created</th>
              <th>Size</th>
              <th>Storage</th>
              <th>Status</th>
              <th className="table-actions">Actions</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((entry) => (
              <tr key={entry.name}>
                <td>
                  {entry.name}
                  {entry.error && (
                    <span className="row-description" role="alert">
                      {entry.error}
                    </span>
                  )}
                </td>
                <td>{new Date(entry.modified_at).toLocaleString()}</td>
                <td>{bytes(entry.size)}</td>
                <td>
                  {[entry.local ? "Local" : null, entry.remote ? "S3" : null]
                    .filter(Boolean)
                    .join(" · ") || "—"}
                </td>
                <td>
                  <BackupState state={entry.state} />
                </td>
                <td className="table-actions">
                  <div className="row-actions">
                    {!busy &&
                      entry.state !== "deleting" &&
                      (entry.remote ||
                        (entry.local && status && !status.busy)) && (
                        <ButtonLink
                          href={`/api/v1/servers/${encodeURIComponent(server.id)}/backups/${encodeURIComponent(entry.name)}`}
                          download
                          iconOnly
                          aria-label={`Download ${entry.name}`}
                          title="Download backup"
                        >
                          <NavigationIcon name="download" />
                        </ButtonLink>
                      )}
                    <Button
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
                    </Button>
                    {canConfigure && entry.local && (
                      <Button
                        variant="danger"
                        disabled={
                          busy ||
                          status?.busy === true ||
                          entry.state === "deleting"
                        }
                        onClick={() => {
                          void remove(entry, "local");
                        }}
                      >
                        Delete local
                      </Button>
                    )}
                    {canConfigure && entry.remote && (
                      <Button
                        variant="danger"
                        disabled={
                          busy ||
                          status?.busy === true ||
                          entry.state === "deleting"
                        }
                        onClick={() => {
                          void remove(entry, "s3");
                        }}
                      >
                        Delete S3
                      </Button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        <p>
          Backups exclude links and special files and are limited to 100 GiB and
          100,000 entries. Manual and scheduled backups use this policy.
          Deleting the server also deletes its local and S3 backups.
        </p>
      </div>
    </Drawer>
  );
}
