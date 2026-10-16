import { useEffect, useRef, useState } from "react";
import { deleteServer, listServers } from "../api/servers";
import type { Server } from "../api/servers";
import { serverOptions, type ServerOptions } from "../api/access";
import { useAuthentication } from "../hooks/useAuthentication";
import { ServerAccessDialog } from "../dialogs/ServerAccessDialog";
import { useCollection } from "../hooks/useCollection";
import { ServerDialog } from "../dialogs/ServerDialog";
import { ServerConsoleDialog } from "../dialogs/ServerConsoleDialog";
import { ServerFilesDialog } from "../dialogs/ServerFilesDialog";
import { ServerBackupsDialog } from "../dialogs/ServerBackupsDialog";
import { ServerLifecycleControls } from "../components/servers/ServerLifecycleControls";
import { useServerRuntime } from "../hooks/useServerRuntime";
import { serverAction } from "../api/runtime";
import type { ServerAction } from "../api/runtime";
import "./management.css";
import { Table } from "../components/table/Table";
import { useMonitoring } from "../hooks/useMonitoring";
import { ResourceReadings } from "../components/servers/ResourceReadings";
import { ServerTransferDialog } from "../dialogs/ServerTransferDialog";

export function ServersPage() {
  const auth = useAuthentication();
  const canApplyRevision =
    auth.session?.admin === true ||
    auth.session?.permissions?.includes("servers.manage_all") === true;
  const [applyRevision, setApplyRevision] = useState(false);
  const canTransfer =
    auth.session?.admin === true ||
    (auth.session?.permissions?.includes("sparks.manage") === true &&
      auth.session.permissions.includes("servers.manage_all"));
  const [transferServer, setTransferServer] = useState<Server | null>(null);
  const canProvision =
    auth.session?.admin === true ||
    auth.session?.permissions?.includes("servers.create") === true;
  const servers = useCollection(listServers);
  const monitoring = useMonitoring();
  const [options, setOptions] = useState<ServerOptions>({
    sparks: [],
    embers: [],
  });
  const [draftOptions, setDraftOptions] = useState<ServerOptions>({
    sparks: [],
    embers: [],
  });
  const [optionsLoading, setOptionsLoading] = useState(canProvision);
  const [optionsError, setOptionsError] = useState<string | null>(null);
  const [optionsRevision, setOptionsRevision] = useState(0);
  const [accessServer, setAccessServer] = useState<Server | null>(null);
  const editorPending = useRef(false);
  useEffect(() => {
    if (!canProvision) return;
    const controller = new AbortController();
    void serverOptions(controller.signal)
      .then((value) => {
        if (!controller.signal.aborted) setOptions(value);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setOptionsError(
            cause instanceof Error
              ? cause.message
              : "Unable to load creation options.",
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setOptionsLoading(false);
      });
    return () => controller.abort();
  }, [canProvision, optionsRevision]);
  const runtime = useServerRuntime(servers.items.map((server) => server.id));
  const [acting, setActing] = useState<string | null>(null);
  const [consoleServer, setConsoleServer] = useState<Server | null>(null);
  const [filesServer, setFilesServer] = useState<Server | null>(null);
  const [backupsServer, setBackupsServer] = useState<Server | null>(null);
  const [editing, setEditing] = useState<Server | null>(null);
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [sparkFilter, setSparkFilter] = useState("");
  const [deleting, setDeleting] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const loading = servers.loading || (canProvision && optionsLoading);
  const error = actionError ?? servers.error ?? optionsError;
  const canCreate =
    canProvision &&
    !loading &&
    options.embers.length > 0 &&
    options.sparks.some(
      (spark) => !spark.disabled && spark.port_allocations.length > 0,
    );
  const visible = servers.items.filter(
    (server) =>
      (!sparkFilter || server.spark_id === sparkFilter) &&
      server.name.toLowerCase().includes(search.toLowerCase()),
  );
  async function refresh() {
    monitoring.refresh();
    setActionError(null);
    setOptionsError(null);
    setOptionsLoading(canProvision);
    await servers.refresh();
    setOptionsRevision((value) => value + 1);
  }
  async function remove(server: Server) {
    if (
      deleting ||
      !window.confirm(
        `Permanently delete "${server.name}", its container, game files and backups, and release its resources and ports?`,
      )
    )
      return;
    setDeleting(server.id);
    setActionError(null);
    try {
      await deleteServer(server.id);
      await servers.refresh();
    } catch (cause) {
      setActionError(
        cause instanceof Error ? cause.message : "Unable to delete server.",
      );
    } finally {
      setDeleting(null);
    }
  }
  function create() {
    setApplyRevision(false);
    setDraftOptions(options);
    setEditing(null);
    setOpen(true);
  }
  async function edit(server: Server, revision = false) {
    if (editorPending.current) return;
    editorPending.current = true;
    setActionError(null);
    try {
      setDraftOptions(
        await serverOptions(AbortSignal.timeout(15000), server.id),
      );
      setEditing(server);
      setApplyRevision(revision);
      setOpen(true);
    } catch (cause) {
      setActionError(
        cause instanceof Error
          ? cause.message
          : "Unable to load server editor.",
      );
    } finally {
      editorPending.current = false;
    }
  }
  async function act(server: Server, action: ServerAction) {
    if (acting !== null) return;
    if (
      (action === "update" || action === "reinstall") &&
      !window.confirm(
        `${action === "update" ? "Update" : "Reinstall"} "${server.name}" using its pinned Ember? ${action === "update" ? "Current images will be pulled. " : ""}The existing game directory and backups are kept, but the installer may overwrite files. Take a backup first. The server stays stopped.`,
      )
    )
      return;
    if (
      action === "kill" &&
      !window.confirm(
        `Force kill "${server.name}"? Unsaved game data may be lost.`,
      )
    )
      return;
    setActing(server.id);
    setActionError(null);
    try {
      runtime.upsert(await serverAction(server.id, action));
    } catch (cause) {
      setActionError(
        cause instanceof Error ? cause.message : "Unable to run server action.",
      );
    } finally {
      setActing(null);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Servers</h1>
          <p className="page-description">
            Game servers, live runtime state, and reserved capacity.
          </p>
        </div>
        <div className="page-actions">
          <button
            className="button"
            disabled={loading}
            onClick={() => {
              void refresh();
            }}
          >
            {loading ? "Refreshing…" : "Refresh"}
          </button>
          {canProvision && (
            <button
              className="button button-primary"
              disabled={!canCreate}
              onClick={create}
            >
              + Create server
            </button>
          )}
        </div>
      </header>
      {error && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
      {!loading && canProvision && !canCreate && (
        <p className="prerequisite-note">
          {auth.session?.admin ? (
            <>
              To create servers, <a href="#embers">import an Ember</a> and{" "}
              <a href="#sparks">
                configure an enabled Spark with port allocations
              </a>
              .
            </>
          ) : (
            "Ask an administrator to approve a Spark for your platform role before creating a server."
          )}
        </p>
      )}
      {servers.items.length > 0 && (
        <div className="list-toolbar">
          <input
            type="search"
            aria-label="Search servers"
            placeholder="Search servers"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <select
            aria-label="Filter by Spark"
            value={sparkFilter}
            onChange={(event) => setSparkFilter(event.target.value)}
          >
            <option value="">All Sparks</option>
            {[
              ...new Map(
                servers.items.map((server) => [
                  server.spark_id,
                  server.spark_name,
                ]),
              ).entries(),
            ].map(([id, name]) => (
              <option key={id} value={id}>
                {name}
              </option>
            ))}
          </select>
        </div>
      )}
      <div className="management-content" aria-busy={loading}>
        {visible.length === 0 ? (
          <div className="empty-state">
            <h2>
              {loading
                ? "Loading servers…"
                : error && servers.items.length === 0
                  ? "Unable to load servers"
                  : servers.items.length
                    ? "No matching servers"
                    : "No servers yet"}
            </h2>
            {canCreate && servers.items.length === 0 && (
              <>
                <p>Create your first server from an Ember.</p>
                <button className="button button-primary" onClick={create}>
                  Create server
                </button>
              </>
            )}
          </div>
        ) : (
          <Table>
            <caption className="visually-hidden">Game servers</caption>
            <thead>
              <tr>
                <th scope="col">Server</th>
                <th scope="col">State</th>
                <th scope="col">Spark / Ember</th>
                <th scope="col">Resources</th>
                <th scope="col">Ports</th>
                <th scope="col">
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {visible.map((server) => (
                <tr key={server.id}>
                  <td>
                    <strong>{server.name}</strong>
                    {server.description && (
                      <span className="row-description">
                        {server.description}
                      </span>
                    )}
                    <span className="row-id">{server.id}</span>
                  </td>
                  <td>
                    <span className="type-badge">
                      {(
                        runtime.statuses[server.id]?.state ?? "unknown"
                      ).replaceAll("_", " ")}
                    </span>
                    {runtime.statuses[server.id]?.state === "running" && (
                      <span className="row-description">
                        {runtime.statuses[server.id]?.ready
                          ? "Ready"
                          : "Waiting for readiness"}
                      </span>
                    )}
                    {runtime.statuses[server.id]?.progress && (
                      <span className="row-description">
                        {runtime.statuses[server.id]?.progress}
                      </span>
                    )}
                    {runtime.statuses[server.id]?.error && (
                      <span className="runtime-error" role="status">
                        {runtime.statuses[server.id]?.error}
                      </span>
                    )}
                  </td>
                  <td>
                    <span>{server.spark_name}</span>
                    <span className="row-description">{server.ember_name}</span>
                  </td>
                  <td>
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
                  </td>
                  <td>
                    {server.ports.map((port, index) => (
                      <span className="resource-line" key={index}>
                        {server.game_address || port.bind_ip}:{port.host_port} /{" "}
                        {port.protocol}
                      </span>
                    ))}
                  </td>
                  <td>
                    <div className="row-actions">
                      {server.permissions.includes("server.operate") && (
                        <ServerLifecycleControls
                          canConfigure={server.permissions.includes(
                            "server.configure",
                          )}
                          status={runtime.statuses[server.id]}
                          disabled={
                            acting !== null ||
                            deleting !== null ||
                            options.sparks.find(
                              (spark) => spark.id === server.spark_id,
                            )?.disabled === true
                          }
                          onAction={(action) => {
                            void act(server, action);
                          }}
                          onConsole={() => setConsoleServer(server)}
                          onFiles={() => setFilesServer(server)}
                          onBackups={() => setBackupsServer(server)}
                        />
                      )}
                      {server.permissions.includes("server.configure") && (
                        <button
                          className="button"
                          disabled={
                            loading ||
                            deleting !== null ||
                            runtime.statuses[server.id]?.busy === true ||
                            !["stopped", "not_installed", "deleted"].includes(
                              runtime.statuses[server.id]?.state ?? "unknown",
                            )
                          }
                          onClick={() => {
                            void edit(server);
                          }}
                          aria-label={`Edit ${server.name}`}
                        >
                          Edit
                        </button>
                      )}
                      {server.permissions.includes("server.delete") && (
                        <button
                          className="button"
                          disabled={
                            deleting !== null ||
                            acting !== null ||
                            runtime.statuses[server.id]?.busy === true
                          }
                          onClick={() => {
                            void remove(server);
                          }}
                          aria-label={`Delete ${server.name}`}
                        >
                          {deleting === server.id ? "Deleting…" : "Delete"}
                        </button>
                      )}
                      {server.permissions.includes("server.share") && (
                        <button
                          className="button"
                          onClick={() => setAccessServer(server)}
                          aria-label={`Access to ${server.name}`}
                        >
                          Access
                        </button>
                      )}
                      {canApplyRevision && (
                        <button
                          className="button"
                          disabled={
                            acting !== null ||
                            deleting !== null ||
                            runtime.statuses[server.id]?.busy === true ||
                            !["stopped", "not_installed", "deleted"].includes(
                              runtime.statuses[server.id]?.state ?? "unknown",
                            )
                          }
                          onClick={() => {
                            void edit(server, true);
                          }}
                          aria-label={`Apply Ember revision to ${server.name}`}
                        >
                          Ember revision
                        </button>
                      )}
                      {canTransfer && (
                        <button
                          className="button"
                          onClick={() => setTransferServer(server)}
                          aria-label={`Transfer ${server.name}`}
                        >
                          Transfer
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </div>
      <p className="list-summary" role="status">
        {loading
          ? "Loading…"
          : `${visible.length} of ${servers.items.length} servers`}
      </p>
      {open && (
        <ServerDialog
          applyRevision={applyRevision}
          server={editing}
          sparks={draftOptions.sparks}
          embers={draftOptions.embers}
          servers={servers.items}
          onClose={() => setOpen(false)}
          onSaved={(server) => {
            servers.upsert(server);
            void refresh();
          }}
        />
      )}
      {accessServer && (
        <ServerAccessDialog
          server={accessServer}
          onClose={() => setAccessServer(null)}
          onChanged={() => {
            void servers.refresh();
          }}
        />
      )}
      {consoleServer && (
        <ServerConsoleDialog
          server={consoleServer}
          onClose={() => setConsoleServer(null)}
        />
      )}
      {filesServer && (
        <ServerFilesDialog
          server={filesServer}
          canWrite={
            ["stopped", "not_installed"].includes(
              runtime.statuses[filesServer.id]?.state ?? "unknown",
            ) && runtime.statuses[filesServer.id]?.busy !== true
          }
          onClose={() => setFilesServer(null)}
        />
      )}
      {backupsServer && (
        <ServerBackupsDialog
          server={backupsServer}
          onClose={() => setBackupsServer(null)}
        />
      )}
      {transferServer && (
        <ServerTransferDialog
          server={
            servers.items.find((item) => item.id === transferServer.id) ??
            transferServer
          }
          onClose={() => setTransferServer(null)}
          onChanged={servers.refresh}
        />
      )}
      <p className="list-summary">
        Game-data allocations use continuous filesystem quotas. Spark requires a
        host filesystem with project quotas enabled before installing or
        starting servers.
      </p>
    </section>
  );
}
