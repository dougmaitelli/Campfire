import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { useState } from "react";
import { SparksTable } from "../components/sparks/SparksTable";
import { SparkConnectionState } from "../components/sparks/SparkConnectionState";
import { SparkDialog } from "../dialogs/SparkDialog";
import { EnrollSparkDialog } from "../dialogs/EnrollSparkDialog";
import type { Spark } from "../api/sparks";
import { deleteSpark } from "../api/sparks";
import { useSparks } from "../hooks/useSparks";
import "./sparks.css";
import "./management.css";
import { SparkDetailsDialog } from "../dialogs/SparkDetailsDialog";

export function SparksPage() {
  const [details, setDetails] = useState<Spark | null>(null);
  const { sparks, loading, error, refresh, addSpark } = useSparks();
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
        <SparksTable
          loading={loading}
          error={error}
          onRetry={() => {
            void refresh();
          }}
          onAdd={openAdd}
          onDetails={setDetails}
          sparks={sparks}
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
          deleteDisabled={deleting !== null}
          actionError={deleteError}
          actions={
            <Button
              type="button"
              variant="warning"
              disabled={details.disabled || deleting !== null}
              onClick={() => {
                setConfirmEnrollment(details);
              }}
            >
              Re-enroll
            </Button>
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
    </section>
  );
}
