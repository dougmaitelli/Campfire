import {
  lazy,
  Suspense,
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react";
import { useTranslation } from "react-i18next";
import { apiFetch, readJSON, API_READ_TIMEOUT_MS } from "../api/client";
import { serverOptions, type ServerOptions } from "../api/access";
import { getCopyTemplate, type CopyTemplate } from "../api/copies";
import { canPerformServer } from "../permissions";
import type { RuntimeStatus } from "../api/runtime";
import {
  deleteServer,
  isServer,
  listServers,
  serverNeedsEmberReview,
  syncServerConfiguration,
  type Server,
} from "../api/servers";
import { RuntimeState } from "../contracts/domains";
import type { ActionMenuItem } from "../components/forms/ActionMenu";
import { ServerState } from "../components/servers/ServerState";
import { useAuthentication } from "./useAuthentication";
import { useConfirmation } from "./useConfirmation";
import { ServerDrawer } from "../drawers/ServerDrawer";
const ServerCopyFlow = lazy(() =>
  import("../drawers/ServerCopyFlow").then((module) => ({
    default: module.ServerCopyFlow,
  })),
);
const ServerTransferDrawer = lazy(() =>
  import("../drawers/ServerTransferDrawer").then((module) => ({
    default: module.ServerTransferDrawer,
  })),
);
const ServerDetailsDrawer = lazy(() =>
  import("../drawers/ServerDetailsDrawer").then((module) => ({
    default: module.ServerDetailsDrawer,
  })),
);

export function useServerManagement(
  server: Server | null,
  status: RuntimeStatus | undefined,
  onSaved: (server: Server) => void,
) {
  const { t } = useTranslation();
  const confirmation = useConfirmation();
  const { session } = useAuthentication();
  const canApplyRevision = canPerformServer(server, "updateEmber", session);
  const canSyncConfiguration = canPerformServer(server, "syncConfiguration");
  const canTransfer = canPerformServer(server, "transfer", session);
  const canConfigure = canPerformServer(server, "configure");
  const canEdit = canPerformServer(server, "edit");
  const canViewJobs = canPerformServer(server, "viewJobs", session);
  const [editor, setEditor] = useState<{
    server: Server;
    options: ServerOptions;
    servers: Server[];
    revision: boolean;
  } | null>(null);
  const [copy, setCopy] = useState<{
    template: CopyTemplate;
    options: ServerOptions;
    servers: Server[];
    mode: "copy" | "quick";
  } | null>(null);
  const [transfer, setTransfer] = useState(false);
  const [jobs, setJobs] = useState(false);
  const [loading, setLoading] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef<AbortController | null>(null);
  useEffect(() => () => pending.current?.abort(), []);
  const serverID = server?.id;
  const refresh = useCallback(async () => {
    if (!serverID) return;
    const value = await readJSON(
      await apiFetch(`/api/v1/servers/${encodeURIComponent(serverID)}`, {
        signal: AbortSignal.timeout(API_READ_TIMEOUT_MS),
      }),
    );
    if (!isServer(value))
      throw new Error(t("serverDetailsDrawer.unexpectedServerDetailsResponse"));
    onSaved(value);
  }, [serverID, onSaved, t]);

  async function prepare(mode: "edit" | "revision" | "copy" | "quick") {
    if (!server || pending.current) return;
    const controller = new AbortController();
    pending.current = controller;
    setLoading(true);
    setError(null);
    const signal = AbortSignal.any([
      controller.signal,
      AbortSignal.timeout(API_READ_TIMEOUT_MS),
    ]);
    try {
      if (mode === "copy" || mode === "quick") {
        const [template, options, servers] = await Promise.all([
          getCopyTemplate(server.id, signal),
          serverOptions(signal),
          listServers(signal),
        ]);
        if (!controller.signal.aborted) {
          setCopy({ template, options, servers, mode });
          setJobs(false);
        }
      } else {
        const [options, servers] = await Promise.all([
          canConfigure || mode === "revision"
            ? serverOptions(signal, server.id)
            : Promise.resolve({ s3_available: false, sparks: [], embers: [] }),
          listServers(signal),
        ]);
        if (!controller.signal.aborted) {
          setEditor({
            server: servers.find((item) => item.id === server.id) ?? server,
            options,
            servers,
            revision: mode === "revision",
          });
          setJobs(false);
        }
      }
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(
          cause instanceof Error
            ? cause.message
            : t(
                mode === "copy" || mode === "quick"
                  ? "serverCopy.unableToLoad"
                  : "serversPage.unableToLoadServerEditor",
              ),
        );
    } finally {
      if (pending.current === controller) {
        pending.current = null;
        setLoading(false);
      }
    }
  }

  async function remove() {
    if (
      !server ||
      deleting ||
      !(await confirmation.confirm({
        title: t("confirmDialog.deleteServer"),
        description: t(
          "serversPage.permanentlyDeleteItsContainerGameFilesAndBackupsAndRelease",
          { serverName: server.name },
        ),
        confirmLabel: t("common.delete"),
        danger: true,
      }))
    )
      return;
    setDeleting(true);
    setError(null);
    try {
      await deleteServer(server.id);
      window.location.hash = "servers";
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : t("serversPage.unableToDeleteServer"),
      );
    } finally {
      setDeleting(false);
    }
  }

  async function syncConfiguration() {
    if (!server || pending.current || !canSyncConfiguration) return;
    const controller = new AbortController();
    pending.current = controller;
    setLoading(true);
    setError(null);
    try {
      await syncServerConfiguration(
        server.id,
        AbortSignal.any([
          controller.signal,
          AbortSignal.timeout(API_READ_TIMEOUT_MS),
        ]),
      );
      if (!controller.signal.aborted) await refresh();
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(
          cause instanceof Error
            ? cause.message
            : t("serversPage.unableToLoadServerEditor"),
        );
    } finally {
      if (pending.current === controller) {
        pending.current = null;
        setLoading(false);
      }
    }
  }

  const busy = loading || deleting;
  const actions: ActionMenuItem[] = [
    ...(server && canEdit
      ? [
          {
            label: t("serverDetailsDrawer.editServer"),
            icon: "rename",
            disabled: busy,
            onSelect: () => void prepare("edit"),
          } satisfies ActionMenuItem,
        ]
      : []),
    ...(canPerformServer(server, "copy", session)
      ? [
          {
            label: t("serverCopy.copy"),
            icon: "servers",
            disabled: busy,
            onSelect: () => void prepare("copy"),
          } satisfies ActionMenuItem,
        ]
      : []),
    ...(canPerformServer(server, "copyFiles", session)
      ? [
          {
            label: t("serverCopy.quickCopy"),
            icon: "servers",
            disabled: busy,
            onSelect: () => void prepare("quick"),
          } satisfies ActionMenuItem,
        ]
      : []),
    ...(server && canApplyRevision && serverNeedsEmberReview(server)
      ? [
          {
            label: t("serversPage.updateEmber"),
            icon: "embers",
            disabled: busy || status?.busy === true,
            onSelect: () => void prepare("revision"),
          } satisfies ActionMenuItem,
        ]
      : []),
    ...(server?.deployment_sync_required && canSyncConfiguration
      ? [
          {
            label: t("emberUpdate.sync"),
            icon: "restart",
            disabled: busy || status?.busy === true,
            onSelect: () => void syncConfiguration(),
          } satisfies ActionMenuItem,
        ]
      : []),
    ...(server && canTransfer
      ? [
          {
            label: t("serversPage.transfer"),
            icon: "arrow-right",
            disabled: busy,
            onSelect: () => {
              setJobs(false);
              setTransfer(true);
            },
          } satisfies ActionMenuItem,
        ]
      : []),
    ...(server && canViewJobs
      ? [
          {
            label: t("common.jobs"),
            icon: "jobs",
            disabled: busy,
            onSelect: () => setJobs(true),
          } satisfies ActionMenuItem,
        ]
      : []),
    ...(canPerformServer(server, "delete")
      ? [
          {
            label: t("common.delete"),
            icon: "delete",
            tone: "danger",
            disabled: busy || status?.busy === true,
            onSelect: () => void remove(),
          } satisfies ActionMenuItem,
        ]
      : []),
  ];

  return {
    actions,
    busy,
    error,
    canApplyRevision,
    canSyncConfiguration,
    syncConfiguration: () => void syncConfiguration(),
    reviewEmber: () => void prepare("revision"),
    dialogs: server && (
      <Suspense fallback={null}>
        {confirmation.dialog}
        {editor && (
          <ServerDrawer
            server={editor.server}
            accessOnly={!canConfigure && !editor.revision}
            applyRevision={editor.revision}
            emberConfiguration={editor.options.ember_configuration}
            emberManagedChanges={editor.options.ember_managed_changes}
            s3Configured={editor.options.s3_available}
            runtimeRestricted={status?.busy === true}
            sparks={editor.options.sparks}
            embers={editor.options.embers}
            servers={editor.servers}
            onClose={() => setEditor(null)}
            onSaved={onSaved}
          />
        )}
        {copy && (
          <ServerCopyFlow
            source={server}
            mode={copy.mode}
            template={copy.template}
            options={copy.options}
            servers={copy.servers}
            onClose={() => setCopy(null)}
            onSaved={(saved) => {
              if (saved.id === server.id) onSaved(saved);
            }}
            onSubmitted={() => {
              setCopy(null);
              setJobs(true);
            }}
          />
        )}
        {transfer && (
          <ServerTransferDrawer
            server={server}
            onClose={() => setTransfer(false)}
            onSubmitted={() => {
              setTransfer(false);
              setJobs(true);
            }}
          />
        )}
        {jobs && (
          <ServerDetailsDrawer
            id={server.id}
            initialTab="jobs"
            onChanged={refresh}
            title={
              <>
                <span>{server.name}</span>
                <ServerState
                  state={status?.state ?? RuntimeState.Unknown}
                  ready={status?.ready ?? true}
                />
              </>
            }
            onClose={() => setJobs(false)}
          />
        )}
      </Suspense>
    ),
  };
}
