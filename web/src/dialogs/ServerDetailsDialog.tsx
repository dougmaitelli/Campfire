import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { useCallback } from "react";
import { apiFetch, readJSON } from "../api/client";
import { isServer, type Server } from "../api/servers";
import { ServerReview } from "../components/servers/ServerReview";
import { serverStatus, type RuntimeStatus } from "../api/runtime";
import { InlineMessage } from "../components/status/InlineMessage";
import { formatErrorMessage } from "../utils/errors";
import {
  ResourceDetailsDialog,
  type ResourceDetailsDialogProps,
} from "./ResourceDetailsDialog";

type Details = Server & { runtime: RuntimeStatus | undefined };

type Props = Omit<
  ResourceDetailsDialogProps<Details>,
  "loadDetails" | "children" | "description" | "editLabel"
> & { id: string };

export function ServerDetailsDialog({ id, ...props }: Props) {
  const links = useResourceLinks();
  const loadDetails = useCallback(
    async (signal: AbortSignal) => {
      const value = await readJSON(
        await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}`, { signal }),
      );
      if (!isServer(value))
        throw new Error("Unexpected server details response.");
      const runtime = await serverStatus(id, signal).catch(() => undefined);
      return { ...value, runtime };
    },
    [id],
  );
  return (
    <ResourceDetailsDialog
      {...props}
      description="Server details and configuration."
      editLabel="Edit server"
      loadDetails={loadDetails}
    >
      {(server) => (
        <>
          <ServerReview
            value={server}
            name={
              <ResourceLink href={links.server(server.id)}>
                {server.name}
              </ResourceLink>
            }
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
                  "Disk quota enforcement is unavailable on this Spark.",
              )}
            </InlineMessage>
          )}
        </>
      )}
    </ResourceDetailsDialog>
  );
}
