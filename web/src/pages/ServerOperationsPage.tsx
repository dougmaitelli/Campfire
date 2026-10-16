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
import "./server-operations.css";

const confirmations = {
  kill: {
    label: "Kill server",
    description:
      "Force the server to stop immediately. Unsaved game data may be lost.",
  },
  update: {
    label: "Update game files",
    description:
      "Pull the current images and run the installer from the pinned Ember. The game directory and backups are kept, but the installer may overwrite files. Take a backup first. The server stays stopped.",
  },
  reinstall: {
    label: "Reinstall server",
    description:
      "Run the installer from the pinned Ember again. The game directory and backups are kept, but the installer may overwrite files. Take a backup first. The server stays stopped.",
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
  const [confirmation, setConfirmation] = useState<ConfirmedAction | null>(
    null,
  );
  const pending = useRef(false);
  const runtime = useServerRuntime(server ? [server.id] : []);
  const monitoring = useMonitoring();
  const status = runtime.statuses[serverID];
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
    setError(null);
    try {
      runtime.upsert(await serverAction(server.id, action));
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
      <header className="page-header management-header">
        <div>
          <a className="operations-back" href="#servers">
            ← Servers
          </a>
          <div className="operations-title">
            <h1 id="page-title">{server?.name ?? "Server"}</h1>
            {server && (
              <ServerState
                state={status?.state ?? "unknown"}
                ready={status?.ready ?? true}
              />
            )}
          </div>
          {server && (
            <p className="page-description">
              {server.description || server.ember_name}
            </p>
          )}
        </div>
        {server && (
          <div className="page-actions">
            {server.permissions.includes("server.operate") && (
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
                  if (
                    action === "kill" ||
                    action === "update" ||
                    action === "reinstall"
                  ) {
                    setError(null);
                    setConfirmation(action);
                  } else void act(action);
                }}
              />
            )}
          </div>
        )}
      </header>
      {error && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
      {loading && <p role="status">Loading server…</p>}
      {server && (
        <>
          <div className="management-content operations-resources">
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
            <div className="operations-addresses">
              {server.ports.map((port, index) => (
                <span key={index}>
                  {server.game_address || port.bind_ip}:{port.host_port} /{" "}
                  {port.protocol}
                </span>
              ))}
            </div>
          </div>
          {monitoring.error && (
            <p className="field-help" role="status">
              {monitoring.error}
            </p>
          )}
          {status?.progress && (
            <p className="field-help" role="status">
              {status.progress}
            </p>
          )}
          {status?.error && (
            <p className="error-message" role="alert">
              {status.error}
            </p>
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
                <ServerConsoleDialog embedded server={server} onClose={back} />
              )}
              {activeTab === "files" && (
                <ServerFilesDialog
                  embedded
                  server={server}
                  canWrite={
                    ["stopped", "not_installed"].includes(
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
          confirmLabel={confirmations[confirmation].label}
          danger={confirmation !== "update"}
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
        />
      )}
    </section>
  );
}
