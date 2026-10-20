import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { Fieldset } from "../components/forms/Fieldset";
import { ContentState } from "../components/layout/ContentState";
import { InlineMessage } from "../components/status/InlineMessage";
import { PortPill } from "../components/network/PortPill";
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
  const activeTab = ["console", "files", "backups"].includes(tab)
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
          <a className="operations-back" href="#servers">
            {t("serverOperationsPage.servers")}
          </a>
        }
        badge={
          server && (
            <ServerState
              state={status?.state ?? "unknown"}
              ready={status?.ready ?? true}
            />
          )
        }
        description={
          server &&
          (server.description || (
            <ResourceLink href={links.ember(server.ember_id)}>
              {server.ember_name}
            </ResourceLink>
          ))
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
          <ContentPanel className="operations-resources">
            <ResourceReadings
              usage={
                monitoring.snapshot?.servers.find(
                  (item) => item.id === server.id,
                )?.usage
              }
              cpuLimit={server.cpu_limit}
              memoryMB={server.memory_mb}
              diskMB={server.disk_mb}
              elapsed={monitoring.elapsed}
              history={resourceHistory?.history}
            />
            <div className="operations-addresses pill-group">
              {server.ports.map((port, index) => (
                <PortPill
                  key={index}
                  address={server.game_address || port.bind_ip}
                  port={port.host_port}
                  protocol={port.protocol}
                />
              ))}
            </div>
            {resourceHistory?.error && (
              <InlineMessage tone="info">{resourceHistory.error}</InlineMessage>
            )}
          </ContentPanel>
          {monitoring.error && (
            <InlineMessage tone="info" role="status">
              {monitoring.error}
            </InlineMessage>
          )}
          {server.permissions.includes("server.operate") ? (
            <>
              <nav
                className="operations-tabs"
                aria-label={t("serverOperationsPage.serverOperations")}
              >
                {(
                  [
                    ["console", t("serverOperationsPage.consoleTab")],
                    ["files", t("serverOperationsPage.filesTab")],
                    ["backups", t("common.backups")],
                  ] as const
                ).map(([item, label]) => (
                  <a
                    key={item}
                    href={`#servers/${encodeURIComponent(server.id)}/${item}`}
                    aria-current={activeTab === item ? "page" : undefined}
                  >
                    {label}
                  </a>
                ))}
              </nav>
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
              <legend>{t("serverOperationsPage.installationMode")}</legend>
              <label>
                <Trans
                  i18nKey="serverOperationsPage.reinstallKeepingFiles"
                  t={t}
                  components={{
                    input: (
                      <input
                        type="radio"
                        name="reinstall-mode"
                        value="keep"
                        checked={reinstallMode === "keep"}
                        onChange={() => setReinstallMode("keep")}
                      />
                    ),
                  }}
                />
              </label>
              <p className="field-help">
                {t(
                  "serverOperationsPage.keepsExistingGameFilesWorldsAndConfigurationTheInstallerMay",
                )}
              </p>
              <label>
                <Trans
                  i18nKey="serverOperationsPage.cleanInstall"
                  t={t}
                  components={{
                    input: (
                      <input
                        type="radio"
                        name="reinstall-mode"
                        value="clean"
                        checked={reinstallMode === "clean"}
                        onChange={() => setReinstallMode("clean")}
                      />
                    ),
                  }}
                />
              </label>
              <p className="field-help">
                {t(
                  "serverOperationsPage.permanentlyDeletesAllGameFilesIncludingWorldsModsAndIn",
                )}
              </p>
            </Fieldset>
          )}
        </ConfirmDialog>
      )}
    </section>
  );
}
