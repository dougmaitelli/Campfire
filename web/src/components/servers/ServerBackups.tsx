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
import { ActionMenu, type ActionMenuItem } from "../forms/ActionMenu";
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
import "./backups.css";
import { ListToolbar } from "../layout/ListToolbar";
import { canPerformServer } from "../../permissions";
import { useNavigationGuard } from "../../hooks/useNavigationGuard";
import { BackupImportDrawer } from "../../drawers/BackupImportDrawer";
import { BackupName } from "./BackupName";
import { BackupMetadataActions } from "./BackupMetadataActions";
import { backupDisplayName } from "../../utils/backupDisplayName";
import { isCurrentOperationSuccessful } from "./operationMessage";
import { ConfirmDialog } from "../layout/ConfirmDialog";
import { CheckboxField } from "../forms/CheckboxField";
import { restoreBackup, type RestorePhase } from "./restoreBackup";
import { useConfirmation } from "../../hooks/useConfirmation";

const restoreProgressKeys = {
  stop: "serverBackups.stoppingForRestore",
  restore: "serverBackups.restoringBackup",
  start: "serverBackups.startingAfterRestore",
} as const;

export function ServerBackups({ server }: { server: Server }) {
  const { t } = useTranslation();
  const confirmation = useConfirmation();
  const [entries, setEntries] = useState<Backup[]>([]);
  const [runtimeError, setRuntimeError] = useState<string | null>(null);
  const canDelete = canPerformServer(server, "deleteBackups");
  const canCreate = canPerformServer(server, "createBackups");
  const canRestore = canPerformServer(server, "restoreBackups");
  const canDownload = canPerformServer(server, "downloadBackups");
  const [status, setStatus] = useState<RuntimeStatus>();
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [currentOperationID, setCurrentOperationID] = useState<string | null>(
    null,
  );
  const [importOpen, setImportOpen] = useState(false);
  const [restore, setRestore] = useState<Backup | null>(null);
  const [restartAfterRestore, setRestartAfterRestore] = useState(true);
  const [restoreError, setRestoreError] = useState<string | null>(null);
  const [restorePhase, setRestorePhase] = useState<RestorePhase | null>(null);
  const [restoreWasRunning, setRestoreWasRunning] = useState(false);
  const [restoredOperationID, setRestoredOperationID] = useState<string | null>(
    null,
  );
  const restoreAbort = useRef<AbortController | null>(null);
  const running = status?.state === RuntimeState.Running;
  const restoreRunning = busy ? restoreWasRunning : running;
  const canOperate = canPerformServer(server, "operate");
  useNavigationGuard(true, busy, false);
  const [refresh, setRefresh] = useState(0);
  const pending = useRef(false);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      restoreAbort.current?.abort();
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
  const canRestoreNow =
    canChange &&
    (status?.state === RuntimeState.Running ||
      status?.state === RuntimeState.Stopped ||
      status?.state === RuntimeState.NotInstalled);
  async function remove(entry: Backup, location: BackupStorage) {
    if (
      pending.current ||
      entry.pinned ||
      !canDelete ||
      !(await confirmation.confirm({
        title: t("confirmDialog.deleteBackup"),
        description: i18n.t(
          "serverBackups.permanentlyDeleteTheCopyOfOtherCopiesAreKeptDeleting",
          {
            locationLocalLocal: location === "local" ? "local" : "S3",
            entryName: backupDisplayName(entry),
          },
        ),
        confirmLabel: t(
          location === "local"
            ? "serverBackups.deleteLocal"
            : "serverBackups.deleteS3",
        ),
        danger: true,
      }))
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
  async function submit(
    action: "backup" | "restore",
    {
      name,
      backupMode = BackupMode.Full,
    }: { name?: string; backupMode?: BackupMode } = {},
  ) {
    if (
      pending.current ||
      !(action === ServerAction.Backup ? canCreateNow : canRestoreNow) ||
      !(action === "backup" ? canCreate : canRestore)
    )
      return;
    if (
      action === "backup" &&
      backupMode === BackupMode.Savedata &&
      !server.savedata_available
    )
      return;
    if (action === "restore" && running && !canOperate) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    setRestoreError(null);
    setCurrentOperationID(null);
    setRestoredOperationID(null);
    setRestoreWasRunning(running);
    const workflow: { phase: RestorePhase | null } = { phase: null };
    try {
      let next: RuntimeStatus;
      if (action === "restore" && name) {
        const controller = new AbortController();
        restoreAbort.current = controller;
        next = await restoreBackup(
          server.id,
          name,
          { stop: running, restart: restartAfterRestore },
          AbortSignal.any([
            controller.signal,
            AbortSignal.timeout(45 * 60 * 1000),
          ]),
          (observed) => {
            if (mounted.current) {
              setStatus(observed);
              setCurrentOperationID(observed.operation_id ?? null);
            }
          },
          (value) => {
            workflow.phase = value;
            setRestorePhase(value);
          },
        );
      } else {
        next = await serverAction(
          server.id,
          action,
          name,
          undefined,
          action === "backup" ? backupMode : undefined,
        );
      }
      if (mounted.current) {
        setCurrentOperationID(next.operation_id ?? null);
        setStatus(next);
        if (action === "restore") {
          setRestoredOperationID(next.operation_id ?? null);
          setRestore(null);
        }
        setRefresh((value) => value + 1);
      }
    } catch (cause) {
      if (mounted.current) {
        const message =
          cause instanceof Error
            ? cause.message
            : i18n.t("serverBackups.unableToSubmitOperation");
        if (workflow.phase === ServerAction.Start) {
          setRestore(null);
          setError(t("serverBackups.restoreRestartFailed", { message }));
        } else if (action === "restore") setRestoreError(message);
        else setError(message);
      }
    } finally {
      pending.current = false;
      restoreAbort.current = null;
      if (mounted.current) {
        setBusy(false);
        setRestorePhase(null);
      }
    }
  }
  return (
    <Panel
      title={t("serverBackups.backups", { serverName: server.name })}
      description={t("serverBackups.description")}
    >
      <div className="panel-content">
        {confirmation.dialog}
        {restore && (
          <ConfirmDialog
            title={t("serverBackups.restoreTitle", { serverName: server.name })}
            description={t(
              restore.mode === BackupMode.Savedata
                ? "backupModes.restoreSavedata"
                : "serverBackups.restoreFull",
            )}
            confirmLabel={
              restoreRunning && canOperate
                ? t(
                    restartAfterRestore
                      ? "serverBackups.stopRestoreRestart"
                      : "serverBackups.stopRestore",
                  )
                : t("serverBackups.restore")
            }
            danger
            busy={busy}
            disabled={!canRestoreNow || (running && !canOperate)}
            error={restoreError}
            onClose={() => setRestore(null)}
            onConfirm={() => {
              void submit("restore", { name: restore.name });
            }}
          >
            <p className="field-help backup-restore-name">
              {t("serverBackups.restoreArchive", {
                backupName: backupDisplayName(restore),
              })}
            </p>
            <p>
              {t(
                restoreRunning
                  ? "serverBackups.restoreRunning"
                  : "serverBackups.restoreStopped",
              )}
            </p>
            {restoreRunning &&
              (canOperate ? (
                <CheckboxField
                  checked={restartAfterRestore}
                  disabled={busy}
                  onChange={(event) =>
                    setRestartAfterRestore(event.target.checked)
                  }
                >
                  {t("serverBackups.restartAfterRestore")}
                </CheckboxField>
              ) : (
                <InlineMessage tone="warning">
                  {t("serverBackups.restoreNeedsOperate")}
                </InlineMessage>
              ))}
            {restorePhase && (
              <InlineMessage tone="info">
                {t(restoreProgressKeys[restorePhase])}
              </InlineMessage>
            )}
          </ConfirmDialog>
        )}
        {importOpen && (
          <BackupImportDrawer
            serverID={server.id}
            onClose={() => setImportOpen(false)}
            onReady={() => {
              setImportOpen(false);
              setRefresh((value) => value + 1);
            }}
          />
        )}
        {canCreate && (
          <ListToolbar className="backup-toolbar">
            <ActionGroup>
              <Button disabled={!canChange} onClick={() => setImportOpen(true)}>
                {t("backupImports.upload")}
              </Button>
              <ActionMenu
                label={t("serverBackups.createBackup")}
                iconOnly={false}
                variant="primary"
                disabled={!canCreateNow}
                items={[
                  {
                    label: t("serverBackups.fullBackup"),
                    icon: "backups",
                    onSelect: () => {
                      void submit("backup", { backupMode: BackupMode.Full });
                    },
                  },
                  ...(server.savedata_available
                    ? [
                        {
                          label: t("backupModes.savedata"),
                          icon: "folder",
                          onSelect: () => {
                            void submit("backup", {
                              backupMode: BackupMode.Savedata,
                            });
                          },
                        } satisfies ActionMenuItem,
                      ]
                    : []),
                ]}
              />
            </ActionGroup>
          </ListToolbar>
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
        {status &&
          !busy &&
          isCurrentOperationSuccessful(status, currentOperationID) &&
          (status.operation === ServerAction.Backup ||
            status.operation === "restore" ||
            restoredOperationID === currentOperationID) && (
            <InlineMessage tone="success">
              {status.operation === "backup"
                ? t("serverBackups.backupCreated")
                : t(
                    status.state === RuntimeState.Running
                      ? "serverBackups.backupRestoredTheServerIsRunning"
                      : "serverBackups.backupRestoredTheServerIsStopped",
                  )}
            </InlineMessage>
          )}
        <Table
          className="server-backups-table table-stacked"
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
                <td className="backup-archive">
                  <BackupName backup={entry} />
                  {entry.error && (
                    <span className="row-description" role="alert">
                      {entry.error}
                    </span>
                  )}
                </td>
                <td className="backup-created">
                  {new Date(entry.modified_at).toLocaleString()}
                </td>
                <td className="backup-size">{formatBytes(entry.size)}</td>
                <td className="backup-storage">
                  {[
                    entry.local ? i18n.t("serverBackups.local") : null,
                    entry.remote ? "S3" : null,
                  ]
                    .filter(Boolean)
                    .join(" · ") || "—"}
                </td>
                <td className="backup-state">
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
                            entryName: backupDisplayName(entry),
                          })}
                          title={i18n.t("serverBackups.downloadBackup")}
                        >
                          <NavigationIcon name="download" />
                        </ButtonLink>
                      )}
                    {canRestore && (
                      <Button
                        iconOnly
                        aria-label={t("serverBackups.restore")}
                        title={t("serverBackups.restore")}
                        disabled={
                          !canRestoreNow ||
                          (status.state === RuntimeState.NotInstalled &&
                            !entry.portable) ||
                          entry.state === BackupStatus.Deleting ||
                          (!entry.local && !entry.remote)
                        }
                        onClick={() => {
                          setRestoreError(null);
                          setRestartAfterRestore(true);
                          setRestore(entry);
                        }}
                      >
                        <NavigationIcon name="restore" />
                      </Button>
                    )}
                    {canDelete && entry.local && (
                      <Button
                        variant="danger"
                        iconOnly
                        aria-label={t("serverBackups.deleteLocal")}
                        title={t("serverBackups.deleteLocal")}
                        disabled={
                          busy ||
                          entry.pinned ||
                          status?.busy === true ||
                          entry.state === BackupStatus.Deleting
                        }
                        onClick={() => {
                          void remove(entry, "local");
                        }}
                      >
                        <NavigationIcon name="delete" />
                      </Button>
                    )}
                    {canDelete && entry.remote && (
                      <Button
                        variant="danger"
                        disabled={
                          busy ||
                          entry.pinned ||
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
                    {canCreate && (
                      <BackupMetadataActions
                        serverID={server.id}
                        backup={entry}
                        disabled={busy || entry.state === BackupStatus.Deleting}
                        onSaved={(updated) => {
                          setEntries((current) =>
                            current.map((item) =>
                              item.name === updated.name ? updated : item,
                            ),
                          );
                          setRefresh((value) => value + 1);
                        }}
                        onError={setError}
                      />
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
