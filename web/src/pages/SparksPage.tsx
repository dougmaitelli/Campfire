import { useState } from "react";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { SparksTable } from "../components/sparks/SparksTable";
import { SparkDialog } from "../dialogs/SparkDialog";
import { EnrollSparkDialog } from "../dialogs/EnrollSparkDialog";
import type { Spark } from "../api/sparks";
import { useSparks } from "../hooks/useSparks";
import "./sparks.css";

export function SparksPage() {
  const { sparks, loading, error, refresh, addSpark } = useSparks();
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Spark | null>(null);
  const [enrolling, setEnrolling] = useState<Spark | null>(null);

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
      {error && (
        <p className="error-message" role="alert">
          {error} Check that the Campfire backend is running, then refresh.
        </p>
      )}
      <div className="sparks-content" aria-busy={loading}>
        {sparks.length > 0 ? (
          <SparksTable
            onEnroll={setEnrolling}
            sparks={sparks}
            onEdit={(spark) => {
              setEditing(spark);
              setAdding(true);
            }}
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
          hasLocalSpark={sparks.some((spark) => spark.kind === "local")}
          onClose={() => setAdding(false)}
          onSaved={(spark) => {
            addSpark(spark);
            void refresh();
          }}
        />
      )}
      {enrolling && (
        <EnrollSparkDialog
          spark={enrolling}
          onClose={() => setEnrolling(null)}
        />
      )}
    </section>
  );
}
