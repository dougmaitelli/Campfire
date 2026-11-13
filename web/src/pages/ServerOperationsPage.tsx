import {
  ReinstallMode,
  OperationState,
  ServerAction,
  RuntimeState,
} from "../contracts/domains";
import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { useResourceLinks } from "../routing/useResourceLinks";
import { Fieldset } from "../components/forms/Fieldset";
import { RadioField } from "../components/forms/RadioField";
import { ContentState } from "../components/layout/ContentState";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  APIError,
  apiFetch,
  readJSON,
  API_READ_TIMEOUT_MS,
} from "../api/client";
import { replaceCurrentRoute } from "../routing/useCurrentPage";
import { isServer, serverNeedsEmberReview, type Server } from "../api/servers";
import { serverAction } from "../api/runtime";
import type { LifecycleAction } from "../contracts/domains";
import { useServerRuntime } from "../hooks/useServerRuntime";
import { useServerManagement } from "../hooks/useServerManagement";
import { useMonitoring } from "../hooks/useMonitoring";
import { useRequestError } from "../hooks/useRequestError";
import { ServerState } from "../components/servers/ServerState";
import { ServerLifecycleControls } from "../components/servers/ServerLifecycleControls";
import { ServerInstallationCard } from "../components/servers/ServerInstallationCard";
import { lifecycleControlState } from "../components/servers/lifecycleControlState";
import { Button } from "../components/forms/Button";
import { ResourceTiles } from "../components/metrics/ResourceTiles";
import { GameStateReadings } from "../components/servers/GameStateReadings";
import { ServerActivity } from "../components/servers/ServerActivity";
import { ResourceGraphs } from "../components/metrics/ResourceGraphs";
import { ServerConsole } from "../components/servers/ServerConsole";
import { ServerFileBrowser } from "../components/servers/ServerFileBrowser";
import { ServerBackups } from "../components/servers/ServerBackups";
import { ServerLogAlerts } from "../components/servers/ServerLogAlerts";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { operationMessage } from "../components/servers/operationMessage";
import "./server-operations.css";
import "../components/navigation/tabs.css";
import { canPerformServer } from "../permissions";

const confirmations = {
  kill: {
    get label() {
      return i18n.t("serverOperationsPage.killServer");
    },
    get description() {
      return i18n.t(
        "serverOperationsPage.forceTheServerToStopImmediatelyUnsavedGameDataMay",
      );
    },
  },
  reinstall: {
    get label() {
      return i18n.t("serverOperationsPage.reinstallServer");
    },
    get description() {
      return i18n.t(
        "serverOperationsPage.pullCurrentImagesAndRunThePinnedEmberSInstaller",
      );
    },
  },
};
type ConfirmedAction = keyof typeof confirmations;

export function ServerOperationsPage({
  serverID,
  tab,
  filePath,
}: {
  serverID: string;
  tab: string;
  filePath?: string | undefined;
}) {
  const { t } = useTranslation();
  const [server, setServer] = useState<Server | null>(null);
  const metadataGeneration = useRef(0);
  const onServerSaved = useCallback((saved: Server) => {
    metadataGeneration.current++;
    setServer(saved);
  }, []);
  const links = useResourceLinks();
  const [error, setError] = useState<string | null>(null);
  const [loadCause, setLoadError] = useState<Error | string | null>(null);
  const { error: loadError } = useRequestError(loadCause);
  const [loading, setLoading] = useState(true);
  const [acting, setActing] = useState(false);
  const [currentOperationID, setCurrentOperationID] = useState<string | null>(
    null,
  );
  const [requestedAction, setRequestedAction] =
    useState<LifecycleAction | null>(null);
  const [reinstallMode, setReinstallMode] = useState<ReinstallMode>(
    ReinstallMode.Keep,
  );
  const [confirmation, setConfirmation] = useState<ConfirmedAction | null>(
    null,
  );
  const pending = useRef(false);
  const runtime = useServerRuntime(server ? [server.id] : []);
  const monitoring = useMonitoring();
  const status = runtime.statuses[serverID];
  const management = useServerManagement(server, status, onServerSaved);
  const usage = monitoring.snapshot?.servers.find(
    (item) => item.id === serverID,
  )?.usage;
  const controlsDisabled =
    acting ||
    management.busy ||
    monitoring.snapshot?.sparks?.some(
      (spark) => spark.id === server?.spark_id && spark.disabled,
    ) === true;
  const controls = lifecycleControlState(status, controlsDisabled);
  const installAttempted =
    requestedAction === ServerAction.Install ||
    status?.operation === ServerAction.Install;
  const message = operationMessage(
    error && server
      ? {
          error,
          error_code: "operation_failed",
          busy: false,
          operation_state: OperationState.Failed,
          ...(requestedAction ? { operation: requestedAction } : {}),
        }
      : status,
    currentOperationID,
  );
  const tabPermissions = {
    console: "readConsole",
    files: "readFiles",
    backups: "readBackups",
    activity: "view",
  } as const;
  const allowedTabs = Object.entries(tabPermissions)
    .filter(([, permission]) => canPerformServer(server, permission))
    .map(([item]) => item);
  const activeTab = allowedTabs.includes(tab) ? tab : allowedTabs[0];
  const showInstallEmpty =
    controls.install &&
    !status?.busy &&
    !installAttempted &&
    activeTab !== "activity";
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let loaded = false;
    const requestedRoute = window.location.hash.slice(1);
    async function load() {
      const generation = metadataGeneration.current;
      try {
        const value = await readJSON(
          await apiFetch(`/api/v1/servers/${encodeURIComponent(serverID)}`, {
            signal: AbortSignal.any([
              controller.signal,
              AbortSignal.timeout(API_READ_TIMEOUT_MS),
            ]),
          }),
        );
        if (!isServer(value))
          throw new Error(
            i18n.t("serverOperationsPage.unexpectedServerResponse"),
          );
        if (
          !controller.signal.aborted &&
          generation === metadataGeneration.current
        ) {
          setServer(value);
          setLoadError(null);
          loaded = true;
        }
      } catch (cause) {
        if (
          !controller.signal.aborted &&
          cause instanceof APIError &&
          cause.status === 404
        ) {
          const currentRoute = window.location.hash.slice(1);
          if (
            currentRoute.split("/").slice(0, 2).join("/") ===
            requestedRoute.split("/").slice(0, 2).join("/")
          ) {
            replaceCurrentRoute("servers", currentRoute);
          }
          return;
        }
        if (!controller.signal.aborted && !loaded)
          setLoadError(
            cause instanceof Error
              ? cause
              : i18n.t("serverOperationsPage.unableToLoadServer"),
          );
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => void load(), 10000);
    }
    void load();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [serverID]);
  async function act(action: LifecycleAction) {
    if (!server || pending.current) return;
    pending.current = true;
    setActing(true);
    setRequestedAction(action);
    setCurrentOperationID(null);
    setError(null);
    if (action === ServerAction.Install)
      window.location.hash = `servers/${encodeURIComponent(server.id)}/console`;
    try {
      const result = await serverAction(
        server.id,
        action,
        undefined,
        action === ServerAction.Reinstall ? reinstallMode : undefined,
      );
      setCurrentOperationID(result.operation_id ?? null);
      runtime.upsert(result);
      setConfirmation(null);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("serverOperationsPage.unableToRunServerAction"),
      );
    } finally {
      pending.current = false;
      setActing(false);
    }
  }
  const installAction = canPerformServer(server, "operate") ? (
    <Button
      variant="primary"
      disabled={controls.blocked || controls.running}
      onClick={() => void act(ServerAction.Install)}
    >
      <NavigationIcon name="download" />
      {t(
        installAttempted
          ? "serverOperationsPage.retryInstallation"
          : "common.install",
      )}
    </Button>
  ) : undefined;
  return (
    <section className="server-operations" aria-labelledby="page-title">
      <PageHeader
        title={server?.name ?? t("common.server")}
        leading={
          <nav className="operations-breadcrumbs">
            <a href="#servers">{t("common.servers")}</a>
            <span aria-hidden="true">›</span>
            <span aria-current="page">
              {server?.name ?? t("common.server")}
            </span>
          </nav>
        }
        badge={
          server && (
            <ServerState
              state={status?.state ?? RuntimeState.Unknown}
              ready={status?.ready ?? true}
              now={
                (monitoring.snapshot?.collected_at ?? 0) + monitoring.elapsed
              }
              startedAt={
                usage?.state === RuntimeState.Running
                  ? usage.started_at
                  : undefined
              }
              stoppedAt={
                usage &&
                [RuntimeState.Stopped, RuntimeState.Exited].some(
                  (value) => value === usage.state,
                )
                  ? usage.stopped_at
                  : undefined
              }
            />
          )
        }
        description={
          server && (
            <span className="operations-references">
              <NavigationIcon name="servers" />
              <ResourceLink href={links.spark(server.spark_id)}>
                {server.spark_name}
              </ResourceLink>
              <span aria-hidden="true">·</span>
              <ResourceLink href={links.ember(server.ember_id)}>
                {server.ember_name}
              </ResourceLink>
            </span>
          )
        }
        actions={
          server && (
            <ServerLifecycleControls
              status={status}
              disabled={controlsDisabled}
              canReinstall={canPerformServer(server, "reinstall")}
              canOperate={
                !showInstallEmpty && canPerformServer(server, "operate")
              }
              actions={management.actions}
              onAction={(action) => {
                if (
                  action === ServerAction.Kill ||
                  action === ServerAction.Reinstall
                ) {
                  setError(null);
                  setReinstallMode(ReinstallMode.Keep);
                  setConfirmation(action);
                } else void act(action);
              }}
            />
          )
        }
      />
      {server &&
        !showInstallEmpty &&
        monitoring.snapshot?.sparks?.some(
          (spark) =>
            spark.id === server.spark_id && spark.agent_update_available,
        ) && (
          <InlineMessage tone="warning">
            <Trans
              i18nKey="serverOperationsPage.thisServerSSparkNodeIsNotRunningTheLatest"
              t={t}
              components={{
                a: (
                  <a href="#sparks">
                    {t("serverOperationsPage.goToSparksToUpdateIt")}
                  </a>
                ),
              }}
            />
          </InlineMessage>
        )}
      {serverNeedsEmberReview(server) && (
        <InlineMessage tone="warning" className="ember-update-notice">
          <strong>{t("emberUpdate.available")}</strong>
          <span>{t("emberUpdate.description")}</span>
          {management.canApplyRevision ? (
            <Button
              disabled={management.busy || status?.busy === true}
              onClick={management.reviewEmber}
            >
              {t("emberUpdate.review")}
            </Button>
          ) : (
            <span>{t("emberUpdate.askAdministrator")}</span>
          )}
        </InlineMessage>
      )}
      {server?.deployment_sync_required && (
        <InlineMessage tone="warning" className="ember-update-notice">
          <strong>{t("emberUpdate.syncRequired")}</strong>
          <span>{t("emberUpdate.syncDescription")}</span>
          {management.canSyncConfiguration && (
            <Button
              disabled={management.busy || status?.busy === true}
              onClick={management.syncConfiguration}
            >
              {t("emberUpdate.sync")}
            </Button>
          )}
        </InlineMessage>
      )}
      {management.error && (
        <InlineMessage role="alert">{management.error}</InlineMessage>
      )}
      {status?.restart_required && (
        <InlineMessage tone="warning" className="ember-update-notice">
          <strong>{t("emberUpdate.restartRequired")}</strong>
          <span>{t("emberUpdate.pendingChanges")}</span>
        </InlineMessage>
      )}
      {management.dialogs}
      {server && (
        <ServerLogAlerts
          key={server.id}
          server={server}
          busy={status?.busy === true}
          monitorError={status?.log_monitor_error}
          onServerSaved={onServerSaved}
        />
      )}
      {loadError && !server && (
        <InlineMessage role="alert">{loadError}</InlineMessage>
      )}
      {loading && (
        <ContentState loading resource={t("serverOperationsPage.server")} />
      )}
      {server &&
        (showInstallEmpty ? (
          <div className="server-install-empty">
            <ServerInstallationCard server={server} action={installAction} />
          </div>
        ) : (
          <>
            <div className="operations-resources">
              <GameStateReadings
                game={
                  monitoring.snapshot?.servers.find(
                    (item) => item.id === serverID,
                  )?.game
                }
                now={
                  (monitoring.snapshot?.collected_at ?? 0) + monitoring.elapsed
                }
                usage={usage}
                elapsed={monitoring.elapsed}
                runtimeState={status?.state}
                busy={status?.busy}
              />
              <ResourceTiles
                usage={usage}
                cpuLimit={server.cpu_limit}
                memoryMB={server.memory_mb}
                diskMB={server.disk_mb}
                elapsed={monitoring.elapsed}
              />
            </div>
            {monitoring.error && (
              <InlineMessage tone="info" role="status">
                {monitoring.error}
              </InlineMessage>
            )}
            <nav
              className="tab-strip operations-tabs"
              aria-label={t("serverOperationsPage.serverOperations")}
            >
              {(
                [
                  ["console", t("serverOperationsPage.consoleTab")],
                  ["files", t("serverOperationsPage.filesTab")],
                  ["backups", t("common.backups")],
                  ["activity", t("serverActivity.title")],
                ] as const
              )
                .filter(([item]) => allowedTabs.includes(item))
                .map(([item, label]) => (
                  <a
                    key={item}
                    href={`#servers/${encodeURIComponent(server.id)}/${item}`}
                    aria-current={activeTab === item ? "page" : undefined}
                  >
                    {label}
                  </a>
                ))}
            </nav>
            {activeTab === "activity" ? (
              <ServerActivity key={server.id} server={server} />
            ) : activeTab ? (
              <>
                {activeTab === "console" && (
                  <>
                    {message && (
                      <InlineMessage tone={message.tone}>
                        <strong>{message.title}</strong>
                        {message.detail && <> {message.detail}</>}
                      </InlineMessage>
                    )}
                    {controls.install &&
                      !acting &&
                      !status?.busy &&
                      installAction && (
                        <div className="server-install-retry">
                          {installAction}
                        </div>
                      )}
                    <ServerConsole server={server} status={status} />
                    <ResourceGraphs
                      serverID={server.id}
                      cpuLimit={server.cpu_limit}
                      memoryMB={server.memory_mb}
                    />
                  </>
                )}
                {activeTab === "files" && (
                  <ServerFileBrowser
                    key={`${server.id}:${filePath ?? ""}`}
                    initialFile={filePath}
                    server={server}
                    canWrite={
                      [
                        RuntimeState.Running,
                        RuntimeState.Stopped,
                        RuntimeState.Exited,
                        RuntimeState.NotInstalled,
                        RuntimeState.Starting,
                        RuntimeState.Stopping,
                        RuntimeState.Restarting,
                      ].some((value) => value === status?.state) &&
                      (status?.file_system_busy ?? status?.busy) !== true
                    }
                  />
                )}
                {activeTab === "backups" && <ServerBackups server={server} />}
              </>
            ) : (
              <p className="field-help">
                {t(
                  "serverOperationsPage.youHaveReadOnlyAccessToThisServerConsoleFiles",
                )}
              </p>
            )}
          </>
        ))}
      {server && confirmation && (
        <ConfirmDialog
          title={t("common.resourceActionTitle", {
            action: confirmations[confirmation].label,
            name: server.name,
          })}
          description={confirmations[confirmation].description}
          confirmLabel={
            confirmation === "reinstall" && reinstallMode === "clean"
              ? t("serverOperationsPage.deleteGameFilesAndReinstall")
              : confirmations[confirmation].label
          }
          danger={confirmation === "kill" || reinstallMode === "clean"}
          warning={confirmation === "reinstall"}
          busy={acting}
          disabled={
            !status ||
            status.busy ||
            (confirmation === "kill"
              ? status.state !== RuntimeState.Running
              : ![
                  RuntimeState.Stopped,
                  RuntimeState.NotInstalled,
                  RuntimeState.Deleted,
                ].some((value) => value === status.state)) ||
            monitoring.snapshot?.sparks?.some(
              (spark) => spark.id === server.spark_id && spark.disabled,
            ) === true
          }
          error={error}
          onClose={() => {
            setConfirmation(null);
            setError(null);
          }}
          onConfirm={() => {
            void act(confirmation);
          }}
        >
          {confirmation === "reinstall" && (
            <Fieldset className="reinstall-options" disabled={acting}>
              <legend className="visually-hidden">
                {t("serverOperationsPage.installationMode")}
              </legend>
              <RadioField
                name="reinstall-mode"
                value="keep"
                checked={reinstallMode === "keep"}
                onChange={() => setReinstallMode(ReinstallMode.Keep)}
                description={t(
                  "serverOperationsPage.keepsExistingGameFilesWorldsAndConfigurationTheInstallerMay",
                )}
              >
                {t("serverOperationsPage.reinstallKeepingFiles")}
              </RadioField>
              <RadioField
                name="reinstall-mode"
                value="clean"
                checked={reinstallMode === "clean"}
                onChange={() => setReinstallMode(ReinstallMode.Clean)}
                tone="danger"
                description={t(
                  "serverOperationsPage.permanentlyDeletesAllGameFilesIncludingWorldsModsAndIn",
                )}
              >
                {t("serverOperationsPage.cleanInstall")}
              </RadioField>
            </Fieldset>
          )}
        </ConfirmDialog>
      )}
    </section>
  );
}
