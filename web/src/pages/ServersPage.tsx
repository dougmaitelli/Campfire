import { Select } from "../components/forms/Select";
import { useEffect, useRef, useState } from "react";
import { deleteServer, listServers } from "../api/servers";
import type { Server } from "../api/servers";
import { serverOptions, type ServerOptions } from "../api/access";
import { useAuthentication } from "../hooks/useAuthentication";
import { ServerAccessDialog } from "../dialogs/ServerAccessDialog";
import { useCollection } from "../hooks/useCollection";
import { ServerDialog } from "../dialogs/ServerDialog";
import { useServerRuntime } from "../hooks/useServerRuntime";
import "./management.css";
import { Table, TableRow } from "../components/table/Table";
import { ResourceDetailsDialog } from "../dialogs/ResourceDetailsDialog";
import { useMonitoring } from "../hooks/useMonitoring";
import { ResourceReadings } from "../components/servers/ResourceReadings";
import { ServerState } from "../components/servers/ServerState";
import { StatusPill } from "../components/status/StatusPill";
import { ServerTransferDialog } from "../dialogs/ServerTransferDialog";

export function ServersPage() {
  const [details, setDetails] = useState<Server | null>(null);
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
    s3_available: false,
    sparks: [],
    embers: [],
  });
  const [draftOptions, setDraftOptions] = useState<ServerOptions>({
    s3_available: false,
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
      setDetails(null);
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
  return (
    <section aria-labelledby="page-title">
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Servers</h1>
          <p className="page-description">
            Server configuration, access, and allocated resources.
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
      {error && servers.items.length > 0 && (
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
          <label>
            Search
            <input
              type="search"
              aria-label="Search servers"
              placeholder="Search servers"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </label>
          <label>
            Spark
            <Select
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
            </Select>
          </label>
        </div>
      )}
      <div className="management-content" aria-busy={loading}>
        <Table
          className="servers-table"
          empty={visible.length === 0}
          loading={servers.loading}
          error={servers.error}
          resource="servers"
          emptyMessage={
            servers.items.length ? "No matching servers" : "No servers yet"
          }
          emptyDescription={
            servers.items.length
              ? "Try a different search or Spark filter."
              : "Create your first server from an Ember."
          }
          emptyAction={
            canCreate && servers.items.length === 0 ? (
              <button className="button button-primary" onClick={create}>
                Create server
              </button>
            ) : null
          }
          onRetry={() => {
            void refresh();
          }}
        >
          <caption className="visually-hidden">Game servers</caption>
          <thead>
            <tr>
              <th scope="col">Server</th>
              <th scope="col">State</th>
              <th scope="col">Spark / Ember</th>
              <th scope="col">Resources</th>
              <th scope="col">Ports</th>
              <th scope="col" className="table-actions">
                <span className="visually-hidden">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {visible.map((server) => (
              <TableRow key={server.id} onOpen={() => setDetails(server)}>
                <td>
                  <button
                    type="button"
                    className="table-row-trigger"
                    onClick={() => setDetails(server)}
                    aria-label={`View details for ${server.name}`}
                  >
                    {server.name}
                  </button>
                  <span className="row-id">{server.id}</span>
                </td>
                <td>
                  <ServerState
                    state={runtime.statuses[server.id]?.state ?? "unknown"}
                    ready={runtime.statuses[server.id]?.ready ?? true}
                  />
                  {runtime.statuses[server.id]?.busy &&
                    runtime.statuses[server.id]?.progress && (
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
                    layout="compact"
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
                  <div className="pill-group">
                    {server.ports.map((port, index) => (
                      <StatusPill key={index} indicator={false}>
                        {server.game_address || port.bind_ip}:{port.host_port} /{" "}
                        {port.protocol.toUpperCase()}
                      </StatusPill>
                    ))}
                  </div>
                </td>
                <td className="table-actions">
                  <div className="row-actions">
                    <a
                      className="button"
                      href={`#servers/${encodeURIComponent(server.id)}/console`}
                    >
                      Open server
                    </a>
                  </div>
                </td>
              </TableRow>
            ))}
          </tbody>
        </Table>
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
          s3Configured={draftOptions.s3_available}
          runtimeRestricted={
            editing !== null && runtime.statuses[editing.id]?.busy === true
          }
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
      {details && (
        <ResourceDetailsDialog
          key={details.id}
          target={{ kind: "server", id: details.id, name: details.name }}
          onDelete={
            details.permissions.includes("server.delete")
              ? () => {
                  void remove(details);
                }
              : undefined
          }
          deleteDisabled={
            deleting !== null || runtime.statuses[details.id]?.busy === true
          }
          deleting={deleting === details.id}
          actionError={actionError}
          actions={
            <>
              <a
                className="button"
                href={`#servers/${encodeURIComponent(details.id)}/console`}
              >
                Open server
              </a>
              {details.permissions.includes("server.share") && (
                <button
                  className="button"
                  disabled={deleting !== null}
                  onClick={() => {
                    setDetails(null);
                    setAccessServer(details);
                  }}
                >
                  Access
                </button>
              )}
              {canApplyRevision && (
                <button
                  className="button"
                  disabled={
                    deleting !== null ||
                    runtime.statuses[details.id]?.busy === true ||
                    !["stopped", "not_installed", "deleted"].includes(
                      runtime.statuses[details.id]?.state ?? "unknown",
                    )
                  }
                  onClick={() => {
                    setDetails(null);
                    void edit(details, true);
                  }}
                >
                  Ember revision
                </button>
              )}
              {canTransfer && (
                <button
                  className="button"
                  disabled={deleting !== null}
                  onClick={() => {
                    setDetails(null);
                    setTransferServer(details);
                  }}
                >
                  Transfer
                </button>
              )}
            </>
          }
          onClose={() => setDetails(null)}
          onEdit={
            details.permissions.includes("server.configure")
              ? () => {
                  setDetails(null);
                  void edit(details);
                }
              : undefined
          }
          editDisabled={loading}
        />
      )}
    </section>
  );
}
