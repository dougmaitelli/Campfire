import { useState } from "react";
import { deleteServer, listServers } from "../api/servers";
import type { Server } from "../api/servers";
import { listEmbers } from "../api/embers";
import { useCollection } from "../hooks/useCollection";
import { useSparks } from "../hooks/useSparks";
import { ServerDialog } from "../dialogs/ServerDialog";
import { ServerConsoleDialog } from "../dialogs/ServerConsoleDialog";
import { ServerFilesDialog } from "../dialogs/ServerFilesDialog";
import { ServerLifecycleControls } from "../components/servers/ServerLifecycleControls";
import { useServerRuntime } from "../hooks/useServerRuntime";
import { serverAction } from "../api/runtime";
import type { ServerAction } from "../api/runtime";
import "./management.css";

export function ServersPage() {
  const servers = useCollection(listServers);
  const runtime = useServerRuntime(servers.items.map((server) => server.id));
  const [acting, setActing] = useState<string | null>(null);
  const [consoleServer, setConsoleServer] = useState<Server | null>(null);
  const [filesServer, setFilesServer] = useState<Server | null>(null);
  const embers = useCollection(listEmbers);
  const sparks = useSparks();
  const [editing, setEditing] = useState<Server | null>(null);
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const [sparkFilter, setSparkFilter] = useState("");
  const [deleting, setDeleting] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const loading = servers.loading || embers.loading || sparks.loading;
  const error = actionError ?? servers.error ?? embers.error ?? sparks.error;
  const canCreate =
    !loading &&
    embers.items.length > 0 &&
    sparks.sparks.some(
      (spark) => !spark.disabled && spark.port_allocations.length > 0,
    );
  const visible = servers.items.filter(
    (server) =>
      (!sparkFilter || server.spark_id === sparkFilter) &&
      server.name.toLowerCase().includes(search.toLowerCase()),
  );
  async function refresh() {
    setActionError(null);
    await Promise.all([servers.refresh(), embers.refresh(), sparks.refresh()]);
  }
  async function remove(server: Server) {
    if (
      deleting ||
      !window.confirm(
        `Permanently delete "${server.name}", its container and game files, and release its resources and ports?`,
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
    setEditing(null);
    setOpen(true);
  }
  async function act(server: Server, action: ServerAction) {
    if (acting !== null) return;
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
          <button
            className="button button-primary"
            disabled={!canCreate}
            onClick={create}
          >
            + Create server
          </button>
        </div>
      </header>
      {error && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
      {!loading && !canCreate && (
        <p className="prerequisite-note">
          To create servers, <a href="#embers">import an Ember</a> and{" "}
          <a href="#sparks">configure an enabled Spark with port allocations</a>
          .
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
            {sparks.sparks.map((spark) => (
              <option key={spark.id} value={spark.id}>
                {spark.name}
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
          <div className="table-container">
            <table className="management-table">
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
                      <span className="row-description">
                        {server.ember_name}
                      </span>
                    </td>
                    <td>
                      <span className="resource-line">
                        {server.cpu_limit} CPU cores
                      </span>
                      <span className="resource-line">
                        {server.memory_mb.toLocaleString()} MiB memory
                      </span>
                      <span className="resource-line">
                        {server.disk_mb.toLocaleString()} MiB disk
                      </span>
                    </td>
                    <td>
                      {server.ports.map((port, index) => (
                        <span className="resource-line" key={index}>
                          {server.game_address || port.bind_ip}:{port.host_port}{" "}
                          / {port.protocol}
                        </span>
                      ))}
                    </td>
                    <td>
                      <div className="row-actions">
                        <ServerLifecycleControls
                          status={runtime.statuses[server.id]}
                          disabled={
                            acting !== null ||
                            deleting !== null ||
                            sparks.sparks.find(
                              (spark) => spark.id === server.spark_id,
                            )?.disabled === true
                          }
                          onAction={(action) => {
                            void act(server, action);
                          }}
                          onConsole={() => setConsoleServer(server)}
                          onFiles={() => setFilesServer(server)}
                        />
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
                            setEditing(server);
                            setOpen(true);
                          }}
                          aria-label={`Edit ${server.name}`}
                        >
                          Edit
                        </button>
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
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <p className="list-summary" role="status">
        {loading
          ? "Loading…"
          : `${visible.length} of ${servers.items.length} servers`}
      </p>
      {open && (
        <ServerDialog
          server={editing}
          sparks={sparks.sparks}
          embers={embers.items}
          servers={servers.items}
          onClose={() => setOpen(false)}
          onSaved={(server) => {
            servers.upsert(server);
            void refresh();
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
      <p className="list-summary">
        Disk allocation is checked during installation and startup. Continuous
        disk quotas are not enabled.
      </p>
    </section>
  );
}
