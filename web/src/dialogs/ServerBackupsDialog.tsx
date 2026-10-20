import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { ActionGroup } from "../components/layout/ActionGroup";
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
  const { t } = useTranslation();
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
              : i18n.t(
                  "serverBackupsDialog.sparkIsUnavailableOffHostBackupsCanStillBeDownloaded",
                ),
          );
          setLoadError(null);
          setEntries(backups.entries);
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setLoadError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverBackupsDialog.unableToLoadBackups"),
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
        i18n.t(
          "serverBackupsDialog.permanentlyDeleteTheCopyOfOtherCopiesAreKeptDeleting",
          {
            locationLocalLocal: location === "local" ? "local" : "S3",
            entryName: entry.name,
          },
        ),
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
          cause instanceof Error
            ? cause.message
            : i18n.t("serverBackupsDialog.unableToDeleteBackup"),
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
        i18n.t(
          "serverBackupsDialog.restoreThisBackupOfThisReplacesItsCurrentGameFiles",
          { serverName: server.name },
        ),
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
            : i18n.t("serverBackupsDialog.unableToSubmitOperation"),
        );
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <Drawer
      embedded={embedded}
      title={t("serverBackupsDialog.backups", { serverName: server.name })}
      description={t(
        "serverBackupsDialog.stopTheServerToCreateOrRestoreABackupCompleted",
      )}
      busy={busy}
      onClose={onClose}
      footerLeft={
        embedded ? null : (
          <Button disabled={busy} onClick={onClose}>
            {t("common.close")}
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
          {t("serverBackupsDialog.createBackup")}
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
                ? t("serverBackupsDialog.backupCreated")
                : t("serverBackupsDialog.backupRestoredTheServerIsStopped")}
            </InlineMessage>
          )}
        <Table
          empty={entries.length === 0}
          loading={loading}
          error={loadError}
          resource={t("serverBackupsDialog.backups2")}
          onRetry={() => {
            setLoading(true);
            setRefresh((value) => value + 1);
          }}
        >
          <caption className="visually-hidden">
            {t("serverBackupsDialog.serverBackups")}
          </caption>
          <thead>
            <tr>
              <th>{t("serverBackupsDialog.archive")}</th>
              <th>{t("serverBackupsDialog.created")}</th>
              <th>{t("serverBackupsDialog.size")}</th>
              <th>{t("serverBackupsDialog.storage")}</th>
              <th>{t("common.status")}</th>
              <th className="table-actions">{t("common.actions")}</th>
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
                  {[
                    entry.local ? i18n.t("serverBackupsDialog.local") : null,
                    entry.remote ? "S3" : null,
                  ]
                    .filter(Boolean)
                    .join(" · ") || "—"}
                </td>
                <td>
                  <BackupState state={entry.state} />
                </td>
                <td className="table-actions">
                  <ActionGroup>
                    {!busy &&
                      entry.state !== "deleting" &&
                      (entry.remote ||
                        (entry.local && status && !status.busy)) && (
                        <ButtonLink
                          href={`/api/v1/servers/${encodeURIComponent(server.id)}/backups/${encodeURIComponent(entry.name)}`}
                          download
                          iconOnly
                          aria-label={i18n.t("serverBackupsDialog.download", {
                            entryName: entry.name,
                          })}
                          title={i18n.t("serverBackupsDialog.downloadBackup")}
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
                      {i18n.t("serverBackupsDialog.restore")}
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
                        {i18n.t("serverBackupsDialog.deleteLocal")}
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
                        {i18n.t("serverBackupsDialog.deleteS3")}
                      </Button>
                    )}
                  </ActionGroup>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        <p>
          {t(
            "serverBackupsDialog.backupsExcludeLinksAndSpecialFilesAndAreLimitedTo",
          )}
        </p>
      </div>
    </Drawer>
  );
}
