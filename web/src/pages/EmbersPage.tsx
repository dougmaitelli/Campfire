import { useState } from "react";
import { listEmbers, type Ember } from "../api/embers";
import { useCollection } from "../hooks/useCollection";
import { ImportEmberDialog } from "../dialogs/ImportEmberDialog";
import "./management.css";
import { Table, TableRow } from "../components/table/Table";
import { StatusPill } from "../components/status/StatusPill";

export function EmbersPage() {
  const { items, loading, error, refresh, upsert } = useCollection(listEmbers);
  const [importing, setImporting] = useState(false);
  const [editing, setEditing] = useState<Ember | null>(null);
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
      {error && items.length > 0 && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
      <div className="management-content" aria-busy={loading}>
        <Table
          empty={items.length === 0}
          loading={loading}
          error={error}
          resource="Embers"
          emptyDescription="Import a game definition before creating servers."
          emptyAction={
            <button
              className="button button-primary"
              onClick={() => setImporting(true)}
            >
              Import Ember
            </button>
          }
          onRetry={() => {
            void refresh();
          }}
        >
          <caption className="visually-hidden">Imported Embers</caption>
          <thead>
            <tr>
              <th scope="col">Game definition</th>
              <th scope="col">Images</th>
              <th scope="col">Required ports</th>
            </tr>
          </thead>
          <tbody>
            {items.map((ember) => (
              <TableRow key={ember.id} onOpen={() => setEditing(ember)}>
                <td>
                  <button
                    type="button"
                    className="table-row-trigger"
                    aria-label={`Edit Ember ${ember.name}`}
                    onClick={() => setEditing(ember)}
                  >
                    {ember.name}
                  </button>
                  {ember.origin && (
                    <span className="row-description">
                      Imported from {ember.origin.format}
                    </span>
                  )}
                  {ember.description && (
                    <span className="row-description">{ember.description}</span>
                  )}
                  <span className="row-id">{ember.id}</span>
                </td>
                <td>{Object.keys(ember.images).join(", ")}</td>
                <td>
                  {ember.required_ports?.length ? (
                    <div className="pill-group">
                      {ember.required_ports.map((port) => (
                        <StatusPill
                          key={`${port.container}/${port.protocol}`}
                          indicator={false}
                        >
                          {port.container} / {port.protocol.toUpperCase()}
                        </StatusPill>
                      ))}
                    </div>
                  ) : (
                    "None"
                  )}
                </td>
              </TableRow>
            ))}
          </tbody>
        </Table>
      </div>
      <p className="list-summary" role="status">
        {loading
          ? "Loading…"
          : `${items.length} ${items.length === 1 ? "Ember" : "Embers"}`}
      </p>
      {(importing || editing) && (
        <ImportEmberDialog
          ember={editing}
          onClose={() => {
            setImporting(false);
            setEditing(null);
          }}
          onImported={(ember) => {
            upsert(ember);
            void refresh();
          }}
          onDeleted={() => {
            void refresh();
          }}
        />
      )}
    </section>
  );
}
