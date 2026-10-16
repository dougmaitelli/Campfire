import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { apiFetch, readJSON } from "../api/client";
import { isServer, type Server } from "../api/servers";
import { isSpark, type Spark } from "../api/sparks";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { ServerReview } from "../components/servers/ServerReview";
import { SparkReview } from "../components/sparks/SparkReview";
import { SparkConnectionState } from "../components/sparks/SparkConnectionState";

export interface ResourceDetailTarget {
  kind: "server" | "spark";
  id: string;
  name: string;
}

type Details =
  { kind: "server"; value: Server } | { kind: "spark"; value: Spark };

export function ResourceDetailsDialog({
  target,
  onClose,
  onEdit,
  editDisabled = false,
  onDelete,
  deleteDisabled = false,
  deleting = false,
  actionError,
  actions,
}: {
  target: ResourceDetailTarget;
  onClose: () => void;
  onEdit?: (() => void) | undefined;
  editDisabled?: boolean;
  onDelete?: (() => void) | undefined;
  deleteDisabled?: boolean;
  deleting?: boolean;
  actionError?: string | null;
  actions?: ReactNode;
}) {
  const [details, setDetails] = useState<Details | null>(null);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try {
        const body = await readJSON(
          await apiFetch(
            `/api/v1/${target.kind === "server" ? "servers" : "sparks"}/${encodeURIComponent(target.id)}`,
            {
              signal: AbortSignal.any([
                controller.signal,
                AbortSignal.timeout(15000),
              ]),
            },
          ),
        );
        if (controller.signal.aborted) return;
        if (target.kind === "server" && isServer(body))
          setDetails({ kind: "server", value: body });
        else if (target.kind === "spark" && isSpark(body))
          setDetails({ kind: "spark", value: body });
        else throw new Error("Unexpected resource details response.");
      } catch (cause) {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : "Unable to load details.",
          );
      }
    }
    void load();
    return () => controller.abort();
  }, [target.kind, target.id]);
  return (
    <Drawer
      title={target.name}
      description={
        target.kind === "server"
          ? "Server details and configuration."
          : "Spark details and configured capacity."
      }
      busy={deleting}
      onClose={onClose}
      footerLeft={
        <div className="row-actions">
          <DrawerDeleteButton
            onDelete={onDelete}
            disabled={!details || deleteDisabled}
            deleting={deleting}
          />
          <button
            type="button"
            className="button"
            disabled={deleting}
            onClick={onClose}
          >
            Close
          </button>
        </div>
      }
      footerRight={
        onEdit ? (
          <button
            type="button"
            className="button button-primary"
            disabled={!details || editDisabled || deleting}
            onClick={onEdit}
          >
            {target.kind === "server" ? "Edit server" : "Edit Spark"}
          </button>
        ) : null
      }
    >
      <div className="drawer-content">
        {actions && <div className="row-actions">{actions}</div>}
        {actionError && (
          <p className="error-message" role="alert">
            {actionError}
          </p>
        )}
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        {!details && !error && <p role="status">Loading details…</p>}
        {details?.kind === "server" && (
          <ServerReview
            value={details.value}
            sparkName={details.value.spark_name}
            emberName={details.value.ember_name}
            notice=""
          />
        )}
        {details?.kind === "spark" && (
          <>
            <SparkConnectionState
              disabled={details.value.disabled}
              connected={details.value.connected}
            />
            <SparkReview
              name={details.value.name}
              description={details.value.description}
              disabled={details.value.disabled}
              settings={details.value}
              notice=""
            />
          </>
        )}
      </div>
    </Drawer>
  );
}
