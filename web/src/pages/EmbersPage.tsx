import { useState } from "react";
import { listEmbers } from "../api/embers";
import { useCollection } from "../hooks/useCollection";
import { ImportEmberDialog } from "../dialogs/ImportEmberDialog";
import "./management.css";

export function EmbersPage() {
  const { items, loading, error, refresh, upsert } = useCollection(listEmbers);
  const [importing, setImporting] = useState(false);
  return (
    <section aria-labelledby="page-title">
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Embers</h1>
          <p className="page-description">
            Game definitions used to configure your servers.
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
          <button
            className="button button-primary"
            onClick={() => setImporting(true)}
          >
            + Import Ember
          </button>
        </div>
      </header>
      {error && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
      <div className="management-content" aria-busy={loading}>
        {items.length === 0 ? (
          <div className="empty-state">
            <h2>
              {loading
                ? "Loading Embers…"
                : error
                  ? "Unable to load Embers"
                  : "No Embers yet"}
            </h2>
            {!loading && !error && (
              <>
                <p>Import a game definition before creating servers.</p>
                <button
                  className="button button-primary"
                  onClick={() => setImporting(true)}
                >
                  Import Ember
                </button>
              </>
            )}
          </div>
        ) : (
          <div className="table-container">
            <table className="management-table">
              <caption className="visually-hidden">Imported Embers</caption>
              <thead>
                <tr>
                  <th scope="col">Game definition</th>
                  <th scope="col">Images</th>
                  <th scope="col">Variables</th>
                  <th scope="col">Required ports</th>
                </tr>
              </thead>
              <tbody>
                {items.map((ember) => (
                  <tr key={ember.id}>
                    <td>
                      <strong>{ember.name}</strong>
                      {ember.origin && (
                        <span className="row-description">
                          Imported from {ember.origin.format}
                        </span>
                      )}
                      {ember.description && (
                        <span className="row-description">
                          {ember.description}
                        </span>
                      )}
                      <span className="row-id">{ember.id}</span>
                    </td>
                    <td>{Object.keys(ember.images).join(", ")}</td>
                    <td>{ember.variables?.length ?? 0}</td>
                    <td>
                      {ember.required_ports?.length
                        ? ember.required_ports
                            .map((port) => `${port.container}/${port.protocol}`)
                            .join(", ")
                        : "None"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <p className="list-summary" role="status">
        {loading
          ? "Loading…"
          : `${items.length} ${items.length === 1 ? "Ember" : "Embers"}`}
      </p>
      {importing && (
        <ImportEmberDialog
          onClose={() => setImporting(false)}
          onImported={(ember) => {
            upsert(ember);
            void refresh();
          }}
        />
      )}
    </section>
  );
}
