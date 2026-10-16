import { useState } from "react";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { SparksTable } from "../components/sparks/SparksTable";
import { SparkDialog } from "../dialogs/SparkDialog";
import { EnrollSparkDialog } from "../dialogs/EnrollSparkDialog";
import { SparkRolesDialog } from "../dialogs/SparkRolesDialog";
import type { Spark } from "../api/sparks";
import { deleteSpark } from "../api/sparks";
import { useSparks } from "../hooks/useSparks";
import "./sparks.css";
import { ResourceDetailsDialog } from "../dialogs/ResourceDetailsDialog";

export function SparksPage() {
  const [details, setDetails] = useState<Spark | null>(null);
  const { sparks, loading, error, refresh, addSpark } = useSparks();
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Spark | null>(null);
  const [enrolling, setEnrolling] = useState<Spark | null>(null);
  const [provisioning, setProvisioning] = useState<Spark | null>(null);
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
      <header className="page-header sparks-header">
        <div>
          <h1 id="page-title">Sparks</h1>
          <p className="page-description">
            The nodes that host your game servers.
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
          <button className="button button-primary" onClick={openAdd}>
            + Add Spark
          </button>
        </div>
      </header>
      {(error ?? deleteError) && (
        <p className="error-message" role="alert">
          {error ?? deleteError}
        </p>
      )}
      <div className="sparks-content" aria-busy={loading}>
        {sparks.length > 0 ? (
          <SparksTable
            onDetails={setDetails}
            onEnroll={setEnrolling}
            onProvisioning={setProvisioning}
            sparks={sparks}
          />
        ) : (
          <div className="empty-state">
            <span className="empty-icon">
              <NavigationIcon name="sparks" />
            </span>
            <h2>
              {loading
                ? "Loading Sparks…"
                : error
                  ? "Unable to load Sparks"
                  : "No Sparks yet"}
            </h2>
            {!loading && !error && (
              <>
                <p>Add your first node to get started.</p>
                <button className="button button-primary" onClick={openAdd}>
                  Add Spark
                </button>
              </>
            )}
          </div>
        )}
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
          }}
        />
      )}
      {provisioning && (
        <SparkRolesDialog
          spark={provisioning}
          onClose={() => setProvisioning(null)}
        />
      )}
      {enrolling && (
        <EnrollSparkDialog
          spark={enrolling}
          onClose={() => setEnrolling(null)}
        />
      )}
      {details && (
        <ResourceDetailsDialog
          key={details.id}
          target={{ kind: "spark", id: details.id, name: details.name }}
          onDelete={() => {
            void remove(details);
          }}
          deleting={deleting === details.id}
          deleteDisabled={deleting !== null}
          actionError={deleteError}
          onClose={() => setDetails(null)}
          onEdit={() => {
            setDetails(null);
            setEditing(details);
            setAdding(true);
          }}
          editDisabled={deleting !== null}
        />
      )}
    </section>
  );
}
