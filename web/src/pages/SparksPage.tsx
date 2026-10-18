import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { useEffect, useState } from "react";
import { SparksTable } from "../components/sparks/SparksTable";
import { SparkConnectionState } from "../components/sparks/SparkConnectionState";
import { SparkDialog } from "../dialogs/SparkDialog";
import { EnrollSparkDialog } from "../dialogs/EnrollSparkDialog";
import type { Spark } from "../api/sparks";
import { deleteSpark, listSparks, updateSparkClient } from "../api/sparks";
import { useSparks } from "../hooks/useSparks";
import "./sparks.css";
import "./management.css";
import { SparkDetailsDialog } from "../dialogs/SparkDetailsDialog";

const UPDATE_RECONNECT_TIMEOUT_MS = 120000;
const UPDATE_STATUS_POLL_INTERVAL_MS = 2000;
const UPDATE_STATUS_REQUEST_TIMEOUT_MS = 10000;

export function SparksPage() {
  const [selectedDetails, setDetails] = useState<Spark | null>(null);
  const { sparks, loading, error, refresh, addSpark } = useSparks();
  const details = selectedDetails
    ? (sparks.find((spark) => spark.id === selectedDetails.id) ??
      selectedDetails)
    : null;
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Spark | null>(null);
  const [enrolling, setEnrolling] = useState<{
    spark: Spark;
    reEnrollment: boolean;
  } | null>(null);
  const [confirmEnrollment, setConfirmEnrollment] = useState<Spark | null>(
    null,
  );
  const [deleting, setDeleting] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [confirmUpdate, setConfirmUpdate] = useState<Spark | null>(null);
  const [updating, setUpdating] = useState<string | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const [waiting, setWaiting] = useState<{
    id: string;
    version: string;
  } | null>(null);
  useEffect(() => {
    if (!waiting) return;
    const controller = new AbortController();
    const deadline = Date.now() + UPDATE_RECONNECT_TIMEOUT_MS;
    let checking = false;
    const timer = window.setInterval(() => {
      if (checking) return;
      checking = true;
      void listSparks(
        AbortSignal.any([
          controller.signal,
          AbortSignal.timeout(UPDATE_STATUS_REQUEST_TIMEOUT_MS),
        ]),
      )
        .then((items) => {
          if (controller.signal.aborted) return;
          const updated = items.find((item) => item.id === waiting.id);
          if (updated?.connected && updated.agent_version === waiting.version) {
            addSpark(updated);
            setDetails((previous) =>
              previous?.id === updated.id ? updated : previous,
            );
            setWaiting(null);
            setUpdating(null);
          } else if (Date.now() > deadline) {
            setUpdateError(
              "The update was accepted, but Spark has not reconnected with the new version yet. Refresh to check its status.",
            );
            setWaiting(null);
            setUpdating(null);
          }
        })
        .catch(() => {
          if (!controller.signal.aborted && Date.now() > deadline) {
            setUpdateError(
              "Unable to confirm the updated Spark connection. Refresh to check its status.",
            );
            setWaiting(null);
            setUpdating(null);
          }
        })
        .finally(() => {
          checking = false;
        });
    }, UPDATE_STATUS_POLL_INTERVAL_MS);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, [waiting, addSpark]);
  async function updateClient(spark: Spark) {
    if (updating || !spark.latest_agent_version) return;
    setUpdating(spark.id);
    setUpdateError(null);
    setConfirmUpdate(null);
    try {
      await updateSparkClient(spark.id, spark.latest_agent_version);
      setWaiting({ id: spark.id, version: spark.latest_agent_version });
    } catch (cause) {
      setUpdateError(
        cause instanceof Error ? cause.message : "Unable to update Spark.",
      );
      setUpdating(null);
    }
  }
  async function remove(spark: Spark) {
    if (
      deleting !== null ||
      !window.confirm(
        `Remove "${spark.name}" and revoke its credentials? Move or delete all its servers and finish transfers first.`,
      )
    )
      return;
    setDeleting(spark.id);
    setDeleteError(null);
    try {
      await deleteSpark(spark.id);
      setDetails(null);
      await refresh();
    } catch (cause) {
      setDeleteError(
        cause instanceof Error ? cause.message : "Unable to remove Spark.",
      );
    } finally {
      setDeleting(null);
    }
  }

  function openAdd() {
    setEditing(null);
    setAdding(true);
  }

  return (
    <section className="page" aria-labelledby="page-title">
      <PageHeader
        title="Sparks"
        description="The nodes that host your game servers."
        actions={
          <>
            <Button
              disabled={loading}
              onClick={() => {
                void refresh();
              }}
              loading={loading}
              loadingLabel={"Refreshing…"}
            >
              {"Refresh"}
            </Button>
            <Button variant="primary" onClick={openAdd}>
              + Add Spark
            </Button>
          </>
        }
      />
      {(deleteError ?? (sparks.length ? error : null)) && (
        <InlineMessage role="alert">{error ?? deleteError}</InlineMessage>
      )}
      <div className="management-content" aria-busy={loading}>
        {updateError && <InlineMessage>{updateError}</InlineMessage>}
        {waiting && (
          <InlineMessage tone="info">
            Spark update installed. Waiting for the client to reconnect…
          </InlineMessage>
        )}
        <SparksTable
          loading={loading}
          error={error}
          onRetry={() => {
            void refresh();
          }}
          onAdd={openAdd}
          onDetails={setDetails}
          sparks={sparks}
          updating={updating}
          onUpdate={setConfirmUpdate}
        />
      </div>
      <p className="list-summary" role="status">
        {loading
          ? "Loading Sparks…"
          : `${sparks.length} ${sparks.length === 1 ? "Spark" : "Sparks"}`}
      </p>
      {adding && (
        <SparkDialog
          spark={editing}
          onClose={() => setAdding(false)}
          onSaved={(spark) => {
            addSpark(spark);
            void refresh();
            if (!editing) setEnrolling({ spark, reEnrollment: false });
          }}
        />
      )}
      {enrolling && (
        <EnrollSparkDialog
          spark={enrolling.spark}
          reEnrollment={enrolling.reEnrollment}
          onClose={() => setEnrolling(null)}
        />
      )}
      {details && (
        <SparkDetailsDialog
          key={details.id}
          id={details.id}
          title={
            <>
              <span>{details.name}</span>
              <SparkConnectionState
                disabled={details.disabled}
                connected={details.connected}
                version={details.agent_version}
              />
            </>
          }
          onDelete={() => {
            void remove(details);
          }}
          deleting={deleting === details.id}
          deleteDisabled={deleting !== null || updating !== null}
          actionError={updateError ?? deleteError}
          actions={
            <>
              {details.agent_update_available && (
                <Button
                  variant="warning"
                  disabled={
                    updating !== null ||
                    !details.capabilities?.includes("spark.self-update.v1")
                  }
                  onClick={() => setConfirmUpdate(details)}
                  loading={updating === details.id}
                  loadingLabel="Updating…"
                >
                  Update Spark
                </Button>
              )}
              <Button
                type="button"
                variant="warning"
                disabled={
                  details.disabled || deleting !== null || updating !== null
                }
                onClick={() => {
                  setConfirmEnrollment(details);
                }}
              >
                Re-enroll
              </Button>
            </>
          }
          onClose={() => setDetails(null)}
          onEdit={() => {
            setDetails(null);
            setEditing(details);
            setAdding(true);
          }}
          editDisabled={deleting !== null}
        />
      )}
      {confirmEnrollment && (
        <ConfirmDialog
          title={`Re-enroll ${confirmEnrollment.name}?`}
          description="Running the re-enrollment command will replace this Spark’s existing credentials. Agents using the old credentials will no longer be able to reconnect. The installer briefly stops and restarts the Spark service, interrupting its connection to Campfire. Server files and configuration are preserved."
          confirmLabel="Continue to re-enrollment"
          warning
          busy={false}
          error={null}
          onClose={() => setConfirmEnrollment(null)}
          onConfirm={() => {
            setEnrolling({ spark: confirmEnrollment, reEnrollment: true });
            setConfirmEnrollment(null);
            setDetails(null);
          }}
        >
          <p>
            Opening the helper or generating a command does not reset
            credentials. They are replaced when the command successfully enrolls
            the agent.
          </p>
        </ConfirmDialog>
      )}
      {confirmUpdate && (
        <ConfirmDialog
          title={`Update ${confirmUpdate.name}?`}
          description={`Install Spark ${confirmUpdate.latest_agent_version ?? "matching this panel"} from this Campfire panel. Its checksum is verified before replacing the binary. The agent briefly disconnects and restarts; game containers keep running. Finish active server operations, uploads, and transfers first.`}
          confirmLabel="Update Spark"
          warning
          busy={false}
          error={null}
          onClose={() => setConfirmUpdate(null)}
          onConfirm={() => {
            void updateClient(confirmUpdate);
          }}
        />
      )}
    </section>
  );
}
