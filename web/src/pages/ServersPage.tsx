import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { PrerequisiteNote } from "../components/status/PrerequisiteNote";
import { ContentPanel } from "../components/layout/ContentPanel";
import { ActionGroup } from "../components/layout/ActionGroup";
import { SearchField } from "../components/forms/SearchField";
import { FormField } from "../components/forms/FormField";
import { Button, ButtonLink } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { ListToolbar } from "../components/layout/ListToolbar";
import { PageHeader } from "../components/layout/PageHeader";
import { Select } from "../components/forms/Select";
import { useEffect, useRef, useState } from "react";
import { deleteServer, listServers } from "../api/servers";
import type { Server } from "../api/servers";
import { serverOptions, type ServerOptions } from "../api/access";
import { useAuthentication } from "../hooks/useAuthentication";
import { useCollection } from "../hooks/useCollection";
import { ServerDialog } from "../dialogs/ServerDialog";
import { useServerRuntime } from "../hooks/useServerRuntime";
import "./servers.css";
import { Table, TableRow } from "../components/table/Table";
import { ServerDetailsDialog } from "../dialogs/ServerDetailsDialog";
import { useMonitoring } from "../hooks/useMonitoring";
import { ResourceReadings } from "../components/servers/ResourceReadings";
import { ServerState } from "../components/servers/ServerState";
import { PortPill } from "../components/network/PortPill";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { ServerTransferDialog } from "../dialogs/ServerTransferDialog";

export function ServersPage() {
  const { t } = useTranslation();
  const links = useResourceLinks();
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
              : i18n.t("serversPage.unableToLoadCreationOptions"),
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
        i18n.t(
          "serversPage.permanentlyDeleteItsContainerGameFilesAndBackupsAndRelease",
          { serverName: server.name },
        ),
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
        cause instanceof Error
          ? cause.message
          : i18n.t("serversPage.unableToDeleteServer"),
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
        server.permissions.includes("server.configure") || revision
          ? await serverOptions(AbortSignal.timeout(15000), server.id)
          : { s3_available: false, sparks: [], embers: [] },
      );
      setEditing(server);
      setApplyRevision(revision);
      setOpen(true);
    } catch (cause) {
      setActionError(
        cause instanceof Error
          ? cause.message
          : i18n.t("serversPage.unableToLoadServerEditor"),
      );
    } finally {
      editorPending.current = false;
    }
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.servers")}
        description={t(
          "serversPage.serverConfigurationAccessAndAllocatedResources",
        )}
        actions={
          <>
            <Button
              disabled={loading}
              onClick={() => {
                void refresh();
              }}
              loading={loading}
              loadingLabel={t("serversPage.refreshing")}
            >
              {t("common.refresh")}
            </Button>
            {canProvision && (
              <Button variant="primary" disabled={!canCreate} onClick={create}>
                {t("serversPage.createServer")}
              </Button>
            )}
          </>
        }
      />
      {error && servers.items.length > 0 && (
        <InlineMessage role="alert">{error}</InlineMessage>
      )}
      {!loading && canProvision && !canCreate && (
        <PrerequisiteNote>
          {auth.session?.admin ? (
            <>
              <Trans
                i18nKey="serversPage.toCreateServersAnd"
                t={t}
                components={{
                  a: <a href="#embers">{t("serversPage.importAnEmber")}</a>,
                  a2: (
                    <a href="#sparks">
                      {t(
                        "serversPage.configureAnEnabledSparkWithPortAllocations",
                      )}
                    </a>
                  ),
                }}
              />
            </>
          ) : (
            t("serversPage.askAnAdministratorToApproveASparkForYourRole")
          )}
        </PrerequisiteNote>
      )}
      {servers.items.length > 0 && (
        <ListToolbar>
          <SearchField
            label={t("common.search")}
            aria-label={t("serversPage.searchServers")}
            placeholder={t("serversPage.searchServers")}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <FormField label={t("common.spark")}>
            <Select
              aria-label={t("serversPage.filterBySpark")}
              value={sparkFilter}
              onValueChange={(selectedValue) => setSparkFilter(selectedValue)}
            >
              <option value="">{t("serversPage.allSparks")}</option>
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
          </FormField>
        </ListToolbar>
      )}
      <ContentPanel aria-busy={loading}>
        <Table
          className="servers-table"
          empty={visible.length === 0}
          loading={servers.loading}
          error={servers.error}
          resource={t("serversPage.servers")}
          emptyMessage={
            servers.items.length
              ? t("serversPage.noMatchingServers")
              : t("serversPage.noServersYet")
          }
          emptyDescription={
            servers.items.length
              ? t("serversPage.tryADifferentSearchOrSparkFilter")
              : t("serversPage.createYourFirstServerFromAnEmber")
          }
          emptyAction={
            canCreate && servers.items.length === 0 ? (
              <Button variant="primary" onClick={create}>
                {t("serversPage.createServer2")}
              </Button>
            ) : null
          }
          onRetry={() => {
            void refresh();
          }}
        >
          <caption className="visually-hidden">
            {t("serversPage.gameServers")}
          </caption>
          <thead>
            <tr>
              <th scope="col">{t("common.server")}</th>
              <th scope="col">{t("serversPage.state")}</th>
              <th scope="col">{t("serversPage.sparkEmber")}</th>
              <th scope="col">{t("serversPage.resources")}</th>
              <th scope="col">{t("common.ports")}</th>
              <th scope="col" className="table-actions">
                <span className="visually-hidden">{t("common.actions")}</span>
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
                    aria-label={i18n.t("serversPage.viewDetailsFor", {
                      serverName: server.name,
                    })}
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
                </td>
                <td>
                  <ResourceLink href={links.spark(server.spark_id)}>
                    {server.spark_name}
                  </ResourceLink>
                  <span className="row-description">
                    <ResourceLink href={links.ember(server.ember_id)}>
                      {server.ember_name}
                    </ResourceLink>
                  </span>
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
                      <PortPill
                        key={index}
                        address={server.game_address || port.bind_ip}
                        port={port.host_port}
                        protocol={port.protocol}
                      />
                    ))}
                  </div>
                </td>
                <td className="table-actions">
                  <ActionGroup>
                    <ButtonLink
                      variant="primary"
                      href={`#servers/${encodeURIComponent(server.id)}/console`}
                      iconOnly
                      aria-label={i18n.t("serversPage.open", {
                        serverName: server.name,
                      })}
                      title={i18n.t("serversPage.openServer")}
                    >
                      <NavigationIcon name="arrow-right" />
                    </ButtonLink>
                  </ActionGroup>
                </td>
              </TableRow>
            ))}
          </tbody>
        </Table>
      </ContentPanel>
      <p className="list-summary" role="status">
        {loading
          ? t("common.loading")
          : t("serversPage.count", {
              count: servers.items.length,
              visible: visible.length,
            })}
      </p>
      {open && (
        <ServerDialog
          applyRevision={applyRevision}
          accessOnly={
            editing !== null &&
            !editing.permissions.includes("server.configure") &&
            !applyRevision
          }
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
        <ServerDetailsDialog
          key={details.id}
          id={details.id}
          title={
            <>
              <span>{details.name}</span>
              <ServerState
                state={runtime.statuses[details.id]?.state ?? "unknown"}
                ready={runtime.statuses[details.id]?.ready ?? true}
              />
            </>
          }
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
              {canApplyRevision && details.ember_update_available && (
                <Button
                  variant="warning"
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
                  {t("serversPage.updateEmber")}
                </Button>
              )}
              {canTransfer && (
                <Button
                  variant="primary"
                  disabled={deleting !== null}
                  onClick={() => {
                    setDetails(null);
                    setTransferServer(details);
                  }}
                >
                  {t("serversPage.transfer")}
                </Button>
              )}
            </>
          }
          onClose={() => setDetails(null)}
          onEdit={
            details.permissions.includes("server.configure") ||
            details.permissions.includes("server.share")
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
