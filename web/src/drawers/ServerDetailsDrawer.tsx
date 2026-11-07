import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { ButtonLink } from "../components/forms/Button";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { useResourceLinks } from "../routing/useResourceLinks";
import { useCallback, useId, useState } from "react";
import { apiFetch, readJSON } from "../api/client";
import { isServer, type Server } from "../api/servers";
import { ServerReview } from "../components/servers/ServerReview";
import { serverStatus, type RuntimeStatus } from "../api/runtime";
import { InlineMessage } from "../components/status/InlineMessage";
import { formatErrorMessage } from "../utils/errors";
import { Tabs } from "../components/navigation/Tabs";
import { ServerJobs } from "../components/servers/ServerJobs";
import { canPerformServer } from "../permissions";
import { useAuthentication } from "../hooks/useAuthentication";
import {
  ResourceDetailsDrawer,
  type ResourceDetailsDrawerProps,
} from "./ResourceDetailsDrawer";

type Details = Server & { runtime: RuntimeStatus | undefined };

type Props = Omit<
  ResourceDetailsDrawerProps<Details>,
  | "loadDetails"
  | "children"
  | "description"
  | "editLabel"
  | "refresh"
  | "headingActions"
> & {
  id: string;
  initialTab?: "details" | "jobs";
  onChanged?: () => Promise<void>;
};

export function ServerDetailsDrawer({
  id,
  initialTab = "details",
  onChanged,
  ...props
}: Props) {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const { session } = useAuthentication();
  const [tab, setTab] = useState(initialTab);
  const [revision, setRevision] = useState(0);
  const tabID = useId();
  const loadDetails = useCallback(
    async (signal: AbortSignal) => {
      const value = await readJSON(
        await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}`, { signal }),
      );
      if (!isServer(value))
        throw new Error(
          i18n.t("serverDetailsDrawer.unexpectedServerDetailsResponse"),
        );
      const runtime = await serverStatus(id, signal).catch(() => undefined);
      return { ...value, runtime };
    },
    [id],
  );
  const jobsChanged = useCallback(async () => {
    setRevision((value) => value + 1);
    await onChanged?.();
  }, [onChanged]);
  return (
    <ResourceDetailsDrawer
      {...props}
      description={t("serverDetailsDrawer.serverDetailsAndConfiguration")}
      editLabel={t("serverDetailsDrawer.editServer")}
      loadDetails={loadDetails}
      refresh={revision}
      headingActions={(server) =>
        canPerformServer(server, "readConsole") && (
          <ButtonLink
            variant="primary"
            href={`#servers/${encodeURIComponent(server.id)}/console`}
            iconOnly
            aria-label={t("serversPage.openConsole", {
              serverName: server.name,
            })}
            title={t("serversPage.console")}
            onClick={(event) => {
              if (
                event.button === 0 &&
                !event.metaKey &&
                !event.ctrlKey &&
                !event.shiftKey &&
                !event.altKey
              )
                props.onClose();
            }}
          >
            <NavigationIcon name="terminal" />
          </ButtonLink>
        )
      }
    >
      {(server) => {
        const canViewJobs = canPerformServer(server, "viewJobs", session);
        return (
          <>
            {canViewJobs && (
              <Tabs
                label={t("serverDetailsDrawer.serverDetailsAndConfiguration")}
                value={tab}
                onChange={setTab}
                items={[
                  {
                    value: "details",
                    label: t("serverDetailsDrawer.details"),
                    id: `${tabID}-details-tab`,
                    panelID: `${tabID}-details`,
                  },
                  {
                    value: "jobs",
                    label: t("serverDetailsDrawer.jobs"),
                    id: `${tabID}-jobs-tab`,
                    panelID: `${tabID}-jobs`,
                  },
                ]}
              />
            )}
            {tab === "jobs" && canViewJobs ? (
              <div
                role="tabpanel"
                id={`${tabID}-jobs`}
                aria-labelledby={`${tabID}-jobs-tab`}
              >
                <ServerJobs server={server} onChanged={jobsChanged} />
              </div>
            ) : (
              <div
                role={canViewJobs ? "tabpanel" : undefined}
                id={`${tabID}-details`}
                aria-labelledby={
                  canViewJobs ? `${tabID}-details-tab` : undefined
                }
              >
                <ServerReview
                  value={server}
                  portDescriptions={server.port_descriptions}
                  variableTypes={server.variable_types}
                  name={server.name}
                  sparkName={
                    <ResourceLink href={links.spark(server.spark_id)}>
                      {server.spark_name}
                    </ResourceLink>
                  }
                  emberName={
                    <ResourceLink href={links.ember(server.ember_id)}>
                      {server.ember_name}
                    </ResourceLink>
                  }
                  notice=""
                />
                {server.runtime?.disk_quota_error_code && (
                  <InlineMessage tone="info">
                    {formatErrorMessage(
                      server.runtime.disk_quota_error ??
                        i18n.t(
                          "serverDetailsDrawer.diskQuotaEnforcementIsUnavailableOnThisSpark",
                        ),
                    )}
                  </InlineMessage>
                )}
              </div>
            )}
          </>
        );
      }}
    </ResourceDetailsDrawer>
  );
}
