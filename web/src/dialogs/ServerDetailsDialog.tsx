import { useCallback } from "react";
import { apiFetch, readJSON } from "../api/client";
import { isServer, type Server } from "../api/servers";
import { ServerReview } from "../components/servers/ServerReview";
import {
  ResourceDetailsDialog,
  type ResourceDetailsDialogProps,
} from "./ResourceDetailsDialog";

type Props = Omit<
  ResourceDetailsDialogProps<Server>,
  "loadDetails" | "children" | "description" | "editLabel"
> & { id: string };

export function ServerDetailsDialog({ id, ...props }: Props) {
  const loadDetails = useCallback(
    async (signal: AbortSignal) => {
      const value = await readJSON(
        await apiFetch(`/api/v1/servers/${encodeURIComponent(id)}`, { signal }),
      );
      if (!isServer(value))
        throw new Error("Unexpected server details response.");
      return value;
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
        <ServerReview
          value={server}
          sparkName={server.spark_name}
          emberName={server.ember_name}
          notice=""
        />
      )}
    </ResourceDetailsDialog>
  );
}
