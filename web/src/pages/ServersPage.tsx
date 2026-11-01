import { RuntimeState } from "../contracts/domains";
import { API_READ_TIMEOUT_MS } from "../api/client";
import { Permission } from "../api/permissions";
import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { PrerequisiteNote } from "../components/status/PrerequisiteNote";
import { ContentPanel } from "../components/layout/ContentPanel";
import { SearchField } from "../components/forms/SearchField";
import { FormField } from "../components/forms/FormField";
import { Button, ButtonLink } from "../components/forms/Button";
import type { ActionMenuItem } from "../components/forms/ActionMenu";
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
import { ServerDrawer } from "../drawers/ServerDrawer";
import { useServerRuntime } from "../hooks/useServerRuntime";
import "./servers.css";
import { Table, TableRow } from "../components/table/Table";
import { ServerDetailsDrawer } from "../drawers/ServerDetailsDrawer";
import { useMonitoring } from "../hooks/useMonitoring";
import { ResourceReadings } from "../components/metrics/ResourceReadings";
import { GameStateReadings } from "../components/servers/GameStateReadings";
import { ServerState } from "../components/servers/ServerState";
import { GameArt } from "../components/media/GameArt";
import "../components/servers/server-table.css";
import { ServerQuickControls } from "../components/servers/ServerQuickControls";
import { PortGroups } from "../components/network/PortGroups";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { ServerTransferDrawer } from "../drawers/ServerTransferDrawer";

const SERVER_LIST_REFRESH_INTERVAL_MS = 10000;

export function ServersPage() {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const [details, setDetails] = useState<Server | null>(null);
  const auth = useAuthentication();
  const canApplyRevision =
    auth.session?.admin === true ||
    auth.session?.permissions?.includes(Permission.ServersManageAll) === true;
  const [applyRevision, setApplyRevision] = useState(false);
  const canTransfer =
    auth.session?.admin === true ||
    (auth.session?.permissions?.includes(Permission.SparksManage) === true &&
      auth.session.permissions.includes(Permission.ServersManageAll));
  const [transferServer, setTransferServer] = useState<Server | null>(null);
  const canProvision =
    auth.session?.admin === true ||
    auth.session?.permissions?.includes(Permission.ServersCreate) === true;
  const servers = useCollection(listServers, SERVER_LIST_REFRESH_INTERVAL_MS);
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
  const hasGameArt = servers.items.some((server) => server.ember_art_url);
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
    options.sparks.some((spark) => !spark.disabled);
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
        server.permissions.includes(Permission.ServerConfigure) || revision
          ? await serverOptions(
              AbortSignal.timeout(API_READ_TIMEOUT_MS),
              server.id,
            )
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
  const detailActions: ActionMenuItem[] = [
    ...(details && canApplyRevision && details.ember_update_available
      ? [
          {
            label: t("serversPage.updateEmber"),
            icon: "embers",
            disabled:
              deleting !== null ||
              runtime.statuses[details.id]?.busy === true ||
              ![
                RuntimeState.Stopped,
                RuntimeState.NotInstalled,
                RuntimeState.Deleted,
              ].some((state) => state === runtime.statuses[details.id]?.state),
            onSelect: () => {
              setDetails(null);
              void edit(details, true);
            },
          } satisfies ActionMenuItem,
        ]
      : []),
    ...(details && canTransfer
      ? [
          {
            label: t("serversPage.transfer"),
            icon: "arrow-right",
            disabled: deleting !== null,
            onSelect: () => {
              setDetails(null);
              setTransferServer(details);
            },
          } satisfies ActionMenuItem,
        ]
      : []),
  ];
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.servers")}
        description={t(
          "serversPage.serverConfigurationAccessAndAllocatedResources",
        )}
        actions={
          <>
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
          className="server-table servers-table table-cards"
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
              {hasGameArt && <th className="game-art-cell" />}
              <th
                scope="col"
                className={
                  hasGameArt ? "table-primary game-art-name" : "table-primary"
                }
              >
                {t("common.server")}
              </th>
              <th scope="col" className="table-state">
                {t("serversPage.state")}
              </th>
              <th scope="col">{t("serversPage.sparkEmber")}</th>
              <th scope="col">{t("serversPage.resources")}</th>
              <th scope="col">{t("common.ports")}</th>
              <th scope="col" className="table-actions">
                <span className="visually-hidden">{t("common.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {visible.map((server) => {
              const monitored = monitoring.snapshot?.servers.find(
                (item) => item.id === server.id,
              );
              const usage = monitored?.usage;
              const gameReadings = (
                <GameStateReadings
                  game={monitored?.game}
                  now={
                    (monitoring.snapshot?.collected_at ?? 0) +
                    monitoring.elapsed
                  }
                  usage={usage}
                  elapsed={monitoring.elapsed}
                  runtimeState={runtime.statuses[server.id]?.state}
                  busy={runtime.statuses[server.id]?.busy}
                />
              );
              return (
                <TableRow key={server.id} onOpen={() => setDetails(server)}>
                  {hasGameArt && (
                    <td className="game-art-cell">
                      <GameArt src={server.ember_art_url} />
                    </td>
                  )}
                  <td
                    className={
                      hasGameArt
                        ? "table-primary game-art-name"
                        : "table-primary"
                    }
                    data-label={t("common.server")}
                  >
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
                    <div className="game-state-table-only">{gameReadings}</div>
                  </td>
                  {monitored?.game && (
                    <td className="game-state-card-cell">{gameReadings}</td>
                  )}
                  <td
                    className="table-state"
                    data-label={t("serversPage.state")}
                  >
                    <ServerState
                      now={
                        (monitoring.snapshot?.collected_at ?? 0) +
                        monitoring.elapsed
                      }
                      state={
                        runtime.statuses[server.id]?.state ??
                        RuntimeState.Unknown
                      }
                      ready={runtime.statuses[server.id]?.ready ?? true}
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
                    {runtime.statuses[server.id]?.busy &&
                      runtime.statuses[server.id]?.progress && (
                        <span className="row-description">
                          {runtime.statuses[server.id]?.progress}
                        </span>
                      )}
                  </td>
                  <td data-label={t("serversPage.sparkEmber")}>
                    <ResourceLink href={links.spark(server.spark_id)}>
                      {server.spark_name}
                    </ResourceLink>
                    <span className="row-description">
                      <ResourceLink href={links.ember(server.ember_id)}>
                        {server.ember_name}
                      </ResourceLink>
                    </span>
                  </td>
                  <td data-label={t("serversPage.resources")}>
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
                  <td data-label={t("common.ports")}>
                    <PortGroups
                      ports={server.ports.map((port) => ({
                        address: server.game_address || port.bind_ip,
                        port: port.host_port,
                        protocol: port.protocol,
                        description:
                          server.port_descriptions?.[
                            `${port.container_port}/${port.protocol}`
                          ],
                      }))}
                    />
                  </td>
                  <td className="table-actions">
                    <ServerQuickControls
                      id={server.id}
                      name={server.name}
                      status={runtime.statuses[server.id]}
                      disabled={
                        deleting === server.id ||
                        monitoring.snapshot?.sparks?.some(
                          (spark) =>
                            spark.id === server.spark_id && spark.disabled,
                        ) === true
                      }
                      canOperate={server.permissions.includes(
                        Permission.ServerOperate,
                      )}
                      onStatus={runtime.upsert}
                      onError={setActionError}
                    >
                      <ButtonLink
                        variant="primary"
                        href={`#servers/${encodeURIComponent(server.id)}/console`}
                        iconOnly
                        aria-label={i18n.t("serversPage.openConsole", {
                          serverName: server.name,
                        })}
                        title={i18n.t("serversPage.console")}
                      >
                        <NavigationIcon name="terminal" />
                      </ButtonLink>
                    </ServerQuickControls>
                  </td>
                </TableRow>
              );
            })}
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
        <ServerDrawer
          applyRevision={applyRevision}
          accessOnly={
            editing !== null &&
            !editing.permissions.includes(Permission.ServerConfigure) &&
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
        <ServerTransferDrawer
          server={
            servers.items.find((item) => item.id === transferServer.id) ??
            transferServer
          }
          onClose={() => setTransferServer(null)}
          onChanged={servers.refresh}
        />
      )}
      {details && (
        <ServerDetailsDrawer
          key={details.id}
          id={details.id}
          title={
            <>
              <span>{details.name}</span>
              <ServerState
                state={
                  runtime.statuses[details.id]?.state ?? RuntimeState.Unknown
                }
                ready={runtime.statuses[details.id]?.ready ?? true}
              />
            </>
          }
          onDelete={
            details.permissions.includes(Permission.ServerDelete)
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
          actions={detailActions}
          onClose={() => setDetails(null)}
          onEdit={
            details.permissions.includes(Permission.ServerConfigure) ||
            details.can_manage_access === true
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
