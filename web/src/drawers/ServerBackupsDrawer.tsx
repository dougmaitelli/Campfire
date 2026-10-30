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
import { Permission } from "../api/permissions";

export function ServerBackupsDrawer({
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
  const canDelete = server.permissions.includes(Permission.ServerBackupsDelete);
  const canCreate = server.permissions.includes(Permission.ServerBackupsCreate);
  const canRestore = server.permissions.includes(
    Permission.ServerBackupsRestore,
  );
  const canDownload = server.permissions.includes(
    Permission.ServerBackupsDownload,
  );
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
                  "serverBackupsDrawer.sparkIsUnavailableOffHostBackupsCanStillBeDownloaded",
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
              : i18n.t("serverBackupsDrawer.unableToLoadBackups"),
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
      !canDelete ||
      !window.confirm(
        i18n.t(
          "serverBackupsDrawer.permanentlyDeleteTheCopyOfOtherCopiesAreKeptDeleting",
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
            : i18n.t("serverBackupsDrawer.unableToDeleteBackup"),
        );
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  async function submit(action: "backup" | "restore", name?: string) {
    if (
      pending.current ||
      !canChange ||
      !(action === "backup" ? canCreate : canRestore)
    )
      return;
    if (
      action === "restore" &&
      !window.confirm(
        i18n.t(
          "serverBackupsDrawer.restoreThisBackupOfThisReplacesItsCurrentGameFiles",
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
            : i18n.t("serverBackupsDrawer.unableToSubmitOperation"),
        );
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <Drawer
      embedded={embedded}
      title={t("serverBackupsDrawer.backups", { serverName: server.name })}
      description={t(
        "serverBackupsDrawer.stopTheServerToCreateOrRestoreABackupCompleted",
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
          disabled={!canChange || !canCreate}
          onClick={() => {
            void submit("backup");
          }}
        >
          {t("serverBackupsDrawer.createBackup")}
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
                ? t("serverBackupsDrawer.backupCreated")
                : t("serverBackupsDrawer.backupRestoredTheServerIsStopped")}
            </InlineMessage>
          )}
        <Table
          empty={entries.length === 0}
          loading={loading}
          error={loadError}
          resource={t("serverBackupsDrawer.backups2")}
          onRetry={() => {
            setLoading(true);
            setRefresh((value) => value + 1);
          }}
        >
          <caption className="visually-hidden">
            {t("serverBackupsDrawer.serverBackups")}
          </caption>
          <thead>
            <tr>
              <th>{t("serverBackupsDrawer.archive")}</th>
              <th>{t("serverBackupsDrawer.created")}</th>
              <th>{t("serverBackupsDrawer.size")}</th>
              <th>{t("serverBackupsDrawer.storage")}</th>
              <th>{t("common.status")}</th>
              <th className="table-actions">
                <span className="visually-hidden">{t("common.actions")}</span>
              </th>
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
                    entry.local ? i18n.t("serverBackupsDrawer.local") : null,
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
                    {canDownload &&
                      !busy &&
                      entry.state !== "deleting" &&
                      (entry.remote ||
                        (entry.local && status && !status.busy)) && (
                        <ButtonLink
                          href={`/api/v1/servers/${encodeURIComponent(server.id)}/backups/${encodeURIComponent(entry.name)}`}
                          download
                          iconOnly
                          aria-label={i18n.t("serverBackupsDrawer.download", {
                            entryName: entry.name,
                          })}
                          title={i18n.t("serverBackupsDrawer.downloadBackup")}
                        >
                          <NavigationIcon name="download" />
                        </ButtonLink>
                      )}
                    <Button
                      disabled={
                        !canChange ||
                        !canRestore ||
                        entry.state === "deleting" ||
                        (!entry.local && !entry.remote)
                      }
                      onClick={() => {
                        void submit("restore", entry.name);
                      }}
                    >
                      {i18n.t("serverBackupsDrawer.restore")}
                    </Button>
                    {canDelete && entry.local && (
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
                        {i18n.t("serverBackupsDrawer.deleteLocal")}
                      </Button>
                    )}
                    {canDelete && entry.remote && (
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
                        {i18n.t("serverBackupsDrawer.deleteS3")}
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
            "serverBackupsDrawer.backupsExcludeLinksAndSpecialFilesAndAreLimitedTo",
          )}
        </p>
      </div>
    </Drawer>
  );
}
