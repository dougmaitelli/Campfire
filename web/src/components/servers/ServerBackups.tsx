import {
  RuntimeState,
  type BackupStorage,
  ServerAction,
  BackupState as BackupStatus,
} from "../../contracts/domains";
import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import { ActionGroup } from "../layout/ActionGroup";
import { Button, ButtonLink } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import { useEffect, useRef, useState } from "react";
import {
  deleteBackup,
  listBackups,
  type Backup,
  BackupMode,
} from "../../api/backups";
import { BackupModeField } from "./BackupModeField";
import {
  serverAction,
  serverStatus,
  type RuntimeStatus,
} from "../../api/runtime";
import type { Server } from "../../api/servers";
import { Panel } from "../layout/Panel";
import { Table } from "../table/Table";
import { BackupState } from "./BackupState";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { formatBytes } from "../../utils/formatBytes";
import "./files.css";
import { Permission } from "../../api/permissions";
import { useNavigationGuard } from "../../hooks/useNavigationGuard";

export function ServerBackups({ server }: { server: Server }) {
  const { t } = useTranslation();
  const [entries, setEntries] = useState<Backup[]>([]);
  const [backupMode, setBackupMode] = useState<BackupMode>(BackupMode.Full);
  const invalidBackupMode =
    backupMode === BackupMode.Savedata && !server.savedata_available;
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
  useNavigationGuard(true, busy, false);
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
                  "serverBackups.sparkIsUnavailableOffHostBackupsCanStillBeDownloaded",
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
              : i18n.t("serverBackups.unableToLoadBackups"),
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
  const canChange = !status?.busy && !busy && loadError === null;
  const canCreateNow =
    canChange &&
    (status?.state === RuntimeState.Running ||
      status?.state === RuntimeState.Stopped);
  const canRestoreNow = canChange && status?.state === RuntimeState.Stopped;
  async function remove(entry: Backup, location: BackupStorage) {
    if (
      pending.current ||
      !canDelete ||
      !window.confirm(
        i18n.t(
          "serverBackups.permanentlyDeleteTheCopyOfOtherCopiesAreKeptDeleting",
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
            : i18n.t("serverBackups.unableToDeleteBackup"),
        );
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  async function submit(action: "backup" | "restore", name?: string) {
    if (
      pending.current ||
      !(action === ServerAction.Backup ? canCreateNow : canRestoreNow) ||
      !(action === "backup" ? canCreate : canRestore)
    )
      return;
    if (action === "backup" && invalidBackupMode) return;
    if (
      action === "restore" &&
      !window.confirm(
        i18n.t(
          entries.find((entry) => entry.name === name)?.mode ===
            BackupMode.Savedata
            ? "backupModes.restoreSavedata"
            : "serverBackups.restoreThisBackupOfThisReplacesItsCurrentGameFiles",
          { serverName: server.name },
        ),
      )
    )
      return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      const next = await serverAction(
        server.id,
        action,
        name,
        undefined,
        action === "backup" ? backupMode : undefined,
      );
      if (mounted.current) {
        setStatus(next);
        setRefresh((value) => value + 1);
      }
    } catch (cause) {
      if (mounted.current)
        setError(
          cause instanceof Error
            ? cause.message
            : i18n.t("serverBackups.unableToSubmitOperation"),
        );
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <Panel
      title={t("serverBackups.backups", { serverName: server.name })}
      description={t("serverBackups.description")}
      footerRight={
        <Button
          variant="primary"
          disabled={!canCreateNow || !canCreate || invalidBackupMode}
          onClick={() => {
            void submit("backup");
          }}
        >
          {t("serverBackups.createBackup")}
        </Button>
      }
    >
      <div className="panel-content">
        {canCreate && (
          <BackupModeField
            value={backupMode}
            onChange={setBackupMode}
            available={server.savedata_available === true}
            disabled={busy || status?.busy === true}
          />
        )}
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
          (status?.operation === ServerAction.Backup ||
            status?.operation === "restore") && (
            <InlineMessage tone="success">
              {status.operation === "backup"
                ? t("serverBackups.backupCreated")
                : t("serverBackups.backupRestoredTheServerIsStopped")}
            </InlineMessage>
          )}
        <Table
          empty={entries.length === 0}
          loading={loading}
          error={loadError}
          resource={t("serverBackups.backups2")}
          onRetry={() => {
            setLoading(true);
            setRefresh((value) => value + 1);
          }}
        >
          <caption className="visually-hidden">
            {t("serverBackups.serverBackups")}
          </caption>
          <thead>
            <tr>
              <th>{t("serverBackups.archive")}</th>
              <th>{t("serverBackups.created")}</th>
              <th>{t("serverBackups.size")}</th>
              <th>{t("serverBackups.storage")}</th>
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
                  <span className="row-description">
                    {t(
                      entry.mode === BackupMode.Savedata
                        ? "backupModes.savedata"
                        : "backupModes.full",
                    )}
                  </span>
                  {entry.error && (
                    <span className="row-description" role="alert">
                      {entry.error}
                    </span>
                  )}
                </td>
                <td>{new Date(entry.modified_at).toLocaleString()}</td>
                <td>{formatBytes(entry.size)}</td>
                <td>
                  {[
                    entry.local ? i18n.t("serverBackups.local") : null,
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
                      entry.state !== BackupStatus.Deleting &&
                      (entry.remote ||
                        (entry.local && status && !status.busy)) && (
                        <ButtonLink
                          href={`/api/v1/servers/${encodeURIComponent(server.id)}/backups/${encodeURIComponent(entry.name)}`}
                          download
                          iconOnly
                          aria-label={i18n.t("serverBackups.download", {
                            entryName: entry.name,
                          })}
                          title={i18n.t("serverBackups.downloadBackup")}
                        >
                          <NavigationIcon name="download" />
                        </ButtonLink>
                      )}
                    <Button
                      disabled={
                        !canRestoreNow ||
                        !canRestore ||
                        entry.state === BackupStatus.Deleting ||
                        (!entry.local && !entry.remote)
                      }
                      onClick={() => {
                        void submit("restore", entry.name);
                      }}
                    >
                      {i18n.t("serverBackups.restore")}
                    </Button>
                    {canDelete && entry.local && (
                      <Button
                        variant="danger"
                        disabled={
                          busy ||
                          status?.busy === true ||
                          entry.state === BackupStatus.Deleting
                        }
                        onClick={() => {
                          void remove(entry, "local");
                        }}
                      >
                        {i18n.t("serverBackups.deleteLocal")}
                      </Button>
                    )}
                    {canDelete && entry.remote && (
                      <Button
                        variant="danger"
                        disabled={
                          busy ||
                          status?.busy === true ||
                          entry.state === BackupStatus.Deleting
                        }
                        onClick={() => {
                          void remove(entry, "s3");
                        }}
                      >
                        {i18n.t("serverBackups.deleteS3")}
                      </Button>
                    )}
                  </ActionGroup>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        <p>
          {t("serverBackups.backupsExcludeLinksAndSpecialFilesAndAreLimitedTo")}
        </p>
      </div>
    </Panel>
  );
}
