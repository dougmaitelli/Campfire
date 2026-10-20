import { ContentPanel } from "../components/layout/ContentPanel";
import { Fieldset } from "../components/forms/Fieldset";
import { ContentState } from "../components/layout/ContentState";
import { InlineMessage } from "../components/status/InlineMessage";
import { PortPill } from "../components/network/PortPill";
import { PageHeader } from "../components/layout/PageHeader";
import { useEffect, useRef, useState } from "react";
import { apiFetch, readJSON } from "../api/client";
import { isServer, type Server } from "../api/servers";
import { serverAction, type ServerAction } from "../api/runtime";
import { useServerRuntime } from "../hooks/useServerRuntime";
import { useMonitoring } from "../hooks/useMonitoring";
import { ServerState } from "../components/servers/ServerState";
import { ServerLifecycleControls } from "../components/servers/ServerLifecycleControls";
import { ResourceReadings } from "../components/servers/ResourceReadings";
import { ServerConsoleDialog } from "../dialogs/ServerConsoleDialog";
import { ServerFilesDialog } from "../dialogs/ServerFilesDialog";
import { ServerBackupsDialog } from "../dialogs/ServerBackupsDialog";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { operationMessage } from "../components/servers/operationMessage";
import "./server-operations.css";

const confirmations = {
  kill: {
    label: "Kill server",
    description:
      "Force the server to stop immediately. Unsaved game data may be lost.",
  },
  reinstall: {
    label: "Reinstall server",
    description:
      "Pull current images and run the pinned Ember’s installer. Choose whether to keep existing game files or start clean. Backups and saved server settings are kept in either case. The server stays stopped.",
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
  const [server, setServer] = useState<Server | null>(null);
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
  useEffect(() => {
    const controller = new AbortController();
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
        if (!isServer(value)) throw new Error("Unexpected server response.");
        if (!controller.signal.aborted) setServer(value);
      } catch (cause) {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : "Unable to load server.",
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
        cause instanceof Error ? cause.message : "Unable to run server action.",
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
        title={server?.name ?? "Server"}
        leading={
          <a className="operations-back" href="#servers">
            ← Servers
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
        description={server && (server.description || server.ember_name)}
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
      {loading && <ContentState loading resource="server" />}
      {server && (
        <>
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
          </ContentPanel>
          {monitoring.error && (
            <InlineMessage tone="info" role="status">
              {monitoring.error}
            </InlineMessage>
          )}
          {message && (
            <InlineMessage tone={message.tone}>
              <strong>{message.title}</strong>
              {message.detail && <> {message.detail}</>}
            </InlineMessage>
          )}
          {server.permissions.includes("server.operate") ? (
            <>
              <nav className="operations-tabs" aria-label="Server operations">
                {["console", "files", "backups"].map((item) => (
                  <a
                    key={item}
                    href={`#servers/${encodeURIComponent(server.id)}/${item}`}
                    aria-current={activeTab === item ? "page" : undefined}
                  >
                    {item.charAt(0).toUpperCase() + item.slice(1)}
                  </a>
                ))}
              </nav>
              {activeTab === "console" && (
                <ServerConsoleDialog
                  embedded
                  server={server}
                  status={status}
                  onClose={back}
                />
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
              You have read-only access to this server. Console, files, and
              backups require permission to operate it.
            </p>
          )}
        </>
      )}
      {server && confirmation && (
        <ConfirmDialog
          title={`${confirmations[confirmation].label}: ${server.name}`}
          description={confirmations[confirmation].description}
          confirmLabel={
            confirmation === "reinstall" && reinstallMode === "clean"
              ? "Delete game files and reinstall"
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
              <legend>Installation mode</legend>
              <label>
                <input
                  type="radio"
                  name="reinstall-mode"
                  value="keep"
                  checked={reinstallMode === "keep"}
                  onChange={() => setReinstallMode("keep")}
                />
                Reinstall keeping files
              </label>
              <p className="field-help">
                Keeps existing game files, worlds and configuration. The
                installer may overwrite files. Take a backup first.
              </p>
              <label>
                <input
                  type="radio"
                  name="reinstall-mode"
                  value="clean"
                  checked={reinstallMode === "clean"}
                  onChange={() => setReinstallMode("clean")}
                />
                Clean install
              </label>
              <p className="field-help">
                Permanently deletes all game files, including worlds, mods and
                in-game configuration, before installing. Existing backups are
                kept. Take a backup first.
              </p>
            </Fieldset>
          )}
        </ConfirmDialog>
      )}
    </section>
  );
}
