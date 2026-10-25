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
import { useEffect, useRef, useState } from "react";
import { APIError, apiFetch, readJSON } from "../api/client";
import { replaceCurrentRoute } from "../routing/useCurrentPage";
import { isServer, type Server } from "../api/servers";
import { serverAction, type ServerAction } from "../api/runtime";
import { useServerRuntime } from "../hooks/useServerRuntime";
import { useMonitoring } from "../hooks/useMonitoring";
import { ServerState } from "../components/servers/ServerState";
import { ServerLifecycleControls } from "../components/servers/ServerLifecycleControls";
import { ResourceReadings } from "../components/servers/ResourceReadings";
import { GameStateReadings } from "../components/servers/GameStateReadings";
import { ServerGameEvents } from "../components/servers/ServerGameEvents";
import { useResourceHistory } from "../hooks/useResourceHistory";
import { ServerConsoleDialog } from "../dialogs/ServerConsoleDialog";
import { ServerFilesDialog } from "../dialogs/ServerFilesDialog";
import { ServerBackupsDialog } from "../dialogs/ServerBackupsDialog";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { operationMessage } from "../components/servers/operationMessage";
import "./server-operations.css";

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
}: {
  serverID: string;
  tab: string;
}) {
  const { t } = useTranslation();
  const [server, setServer] = useState<Server | null>(null);
  const links = useResourceLinks();
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [acting, setActing] = useState(false);
  const [currentOperationID, setCurrentOperationID] = useState<string | null>(
    null,
  );
  const [requestedAction, setRequestedAction] = useState<ServerAction | null>(
    null,
  );
  const [reinstallMode, setReinstallMode] = useState<"keep" | "clean">("keep");
  const [confirmation, setConfirmation] = useState<ConfirmedAction | null>(
    null,
  );
  const pending = useRef(false);
  const runtime = useServerRuntime(server ? [server.id] : []);
  const monitoring = useMonitoring();
  const status = runtime.statuses[serverID];
  const usage = monitoring.snapshot?.servers.find(
    (item) => item.id === serverID,
  )?.usage;
  const message = operationMessage(
    error && server
      ? {
          error,
          error_code: "operation_failed",
          busy: false,
          operation_state: "failed",
          ...(requestedAction ? { operation: requestedAction } : {}),
        }
      : status,
    currentOperationID,
  );
  const activeTab =
    ["console", "files", "backups", "activity"].includes(tab) &&
    (tab !== "activity" || server?.game_integration === true)
      ? tab
      : "console";
  const resourceHistory = useResourceHistory(server?.id);
  useEffect(() => {
    const controller = new AbortController();
    const requestedRoute = window.location.hash.slice(1);
    async function load() {
      try {
        const value = await readJSON(
          await apiFetch(`/api/v1/servers/${encodeURIComponent(serverID)}`, {
            signal: AbortSignal.any([
              controller.signal,
              AbortSignal.timeout(15000),
            ]),
          }),
        );
        if (!isServer(value))
          throw new Error(
            i18n.t("serverOperationsPage.unexpectedServerResponse"),
          );
        if (!controller.signal.aborted) setServer(value);
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
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverOperationsPage.unableToLoadServer"),
          );
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
    }
    void load();
    return () => controller.abort();
  }, [serverID]);
  async function act(action: ServerAction) {
    if (!server || pending.current) return;
    pending.current = true;
    setActing(true);
    setRequestedAction(action);
    setCurrentOperationID(null);
    setError(null);
    try {
      const result = await serverAction(
        server.id,
        action,
        undefined,
        action === "reinstall" ? reinstallMode : undefined,
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
  const back = () => {
    window.location.hash = "servers";
  };
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
              state={status?.state ?? "unknown"}
              ready={status?.ready ?? true}
              now={
                (monitoring.snapshot?.collected_at ?? 0) + monitoring.elapsed
              }
              startedAt={
                usage?.state === "running" ? usage.started_at : undefined
              }
              stoppedAt={
                usage && ["stopped", "exited"].includes(usage.state)
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
          server?.permissions.includes("server.operate") && (
            <ServerLifecycleControls
              status={status}
              disabled={
                acting ||
                monitoring.snapshot?.sparks?.some(
                  (spark) => spark.id === server.spark_id && spark.disabled,
                ) === true
              }
              canConfigure={server.permissions.includes("server.configure")}
              onAction={(action) => {
                if (action === "kill" || action === "reinstall") {
                  setError(null);
                  setReinstallMode("keep");
                  setConfirmation(action);
                } else void act(action);
              }}
            />
          )
        }
      />
      {error && !server && <InlineMessage role="alert">{error}</InlineMessage>}
      {loading && (
        <ContentState loading resource={t("serverOperationsPage.server")} />
      )}
      {server && (
        <>
          {monitoring.snapshot?.sparks?.some(
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
            <ResourceReadings
              usage={usage}
              cpuLimit={server.cpu_limit}
              memoryMB={server.memory_mb}
              diskMB={server.disk_mb}
              elapsed={monitoring.elapsed}
              history={resourceHistory?.history}
            />
            {resourceHistory?.error && (
              <InlineMessage tone="info">{resourceHistory.error}</InlineMessage>
            )}
          </div>
          {monitoring.error && (
            <InlineMessage tone="info" role="status">
              {monitoring.error}
            </InlineMessage>
          )}
          <nav
            className="operations-tabs"
            aria-label={t("serverOperationsPage.serverOperations")}
          >
            {(
              [
                ["console", t("serverOperationsPage.consoleTab")],
                ["files", t("serverOperationsPage.filesTab")],
                ["backups", t("common.backups")],
                ["activity", t("gameEvents.title")],
              ] as const
            )
              .filter(([item]) =>
                item === "activity"
                  ? server.game_integration === true
                  : server.permissions.includes("server.operate"),
              )
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
            <ServerGameEvents key={server.id} serverID={server.id} />
          ) : server.permissions.includes("server.operate") ? (
            <>
              {activeTab === "console" && (
                <>
                  {message && (
                    <InlineMessage tone={message.tone}>
                      <strong>{message.title}</strong>
                      {message.detail && <>{message.detail}</>}
                    </InlineMessage>
                  )}
                  <ServerConsoleDialog
                    embedded
                    server={server}
                    status={status}
                    onClose={back}
                  />
                </>
              )}
              {activeTab === "files" && (
                <ServerFilesDialog
                  embedded
                  server={server}
                  canWrite={
                    ["running", "stopped", "not_installed"].includes(
                      status?.state ?? "unknown",
                    ) && status?.busy !== true
                  }
                  onClose={back}
                />
              )}
              {activeTab === "backups" && (
                <ServerBackupsDialog embedded server={server} onClose={back} />
              )}
            </>
          ) : (
            <p className="field-help">
              {t(
                "serverOperationsPage.youHaveReadOnlyAccessToThisServerConsoleFiles",
              )}
            </p>
          )}
        </>
      )}
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
              ? status.state !== "running"
              : !["stopped", "not_installed", "deleted"].includes(
                  status.state,
                )) ||
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
                onChange={() => setReinstallMode("keep")}
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
                onChange={() => setReinstallMode("clean")}
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
