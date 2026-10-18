import { useEffect, useState } from "react";
import {
  listEmbers,
  listEmberUpdates,
  checkEmberUpdate,
  downloadEmberUpdate,
  type Ember,
  type EmberPreview,
} from "../api/embers";
import { useCollection } from "../hooks/useCollection";
import { ImportEmberDialog } from "../dialogs/ImportEmberDialog";
import "./management.css";
import { Table, TableRow } from "../components/table/Table";
import { StatusPill } from "../components/status/StatusPill";

export function EmbersPage() {
  const { items, loading, error, refresh, upsert } = useCollection(listEmbers);
  const [importing, setImporting] = useState(false);
  const [editing, setEditing] = useState<Ember | null>(null);
  const checks = useCollection(listEmberUpdates);
  const [busy, setBusy] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [update, setUpdate] = useState<{
    preview: EmberPreview;
    url: string;
  } | null>(null);
  const refreshChecks = checks.refresh;
  useEffect(() => {
    const timer = window.setInterval(() => {
      void refreshChecks();
    }, 60000);
    return () => window.clearInterval(timer);
  }, [refreshChecks]);
  async function check(ember: Ember, download = false) {
    if (busy) return;
    setBusy(ember.id);
    setActionError(null);
    try {
      if (download) {
        const preview = await downloadEmberUpdate(ember.id);
        setUpdate({
          preview,
          url: ember.updateUrl ?? ember.origin?.update_url ?? "",
        });
        setImporting(true);
        await refreshChecks();
      } else {
        checks.upsert(await checkEmberUpdate(ember.id));
      }
    } catch (cause) {
      setActionError(
        cause instanceof Error ? cause.message : "Unable to check for updates.",
      );
    } finally {
      setBusy(null);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Embers</h1>
          <p className="page-description">
            Game definitions used to configure your servers. Update URLs are
            checked daily. Download updates to review and import a new revision.
          </p>
        </div>
        <div className="page-actions">
          <button
            className="button"
            disabled={loading}
            onClick={() => {
              void refresh();
              void refreshChecks();
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
      {(actionError ?? checks.error) && (
        <p className="error-message" role="alert">
          {actionError ?? checks.error}
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
              <th scope="col">Updates</th>
              <th scope="col">Images</th>
              <th scope="col">Required ports</th>
            </tr>
          </thead>
          <tbody>
            {items.map((ember) => {
              const status = checks.items.find(
                (item) => item.ember_id === ember.id,
              );
              const updateURL = ember.updateUrl ?? ember.origin?.update_url;
              return (
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
                      <span className="row-description">
                        {ember.description}
                      </span>
                    )}
                    <span className="row-id">{ember.id}</span>
                  </td>
                  <td>
                    {updateURL ? (
                      <>
                        <StatusPill
                          tone={
                            status?.error
                              ? "negative"
                              : status?.available
                                ? "warning"
                                : status
                                  ? "positive"
                                  : "neutral"
                          }
                        >
                          {status?.error
                            ? "Check failed"
                            : status?.available
                              ? "Update available"
                              : status
                                ? "Up to date"
                                : "Not checked yet"}
                        </StatusPill>
                        <span className="row-description">
                          {status ? (
                            <>
                              Last checked:{" "}
                              <time
                                dateTime={new Date(
                                  status.checked_at * 1000,
                                ).toISOString()}
                              >
                                {new Date(
                                  status.checked_at * 1000,
                                ).toLocaleString()}
                              </time>
                            </>
                          ) : (
                            "Checks run automatically every day."
                          )}
                        </span>
                        {status?.error && (
                          <span className="row-description">
                            {status.error}
                          </span>
                        )}
                        <div className="row-actions">
                          <button
                            className="button"
                            disabled={busy !== null}
                            onClick={() => {
                              void check(ember);
                            }}
                          >
                            {busy === ember.id ? "Working…" : "Check now"}
                          </button>
                          {status?.available && !status.error && (
                            <button
                              className="button button-primary"
                              disabled={busy !== null}
                              onClick={() => {
                                void check(ember, true);
                              }}
                            >
                              Download update
                            </button>
                          )}
                        </div>
                      </>
                    ) : (
                      "No update URL"
                    )}
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
              );
            })}
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
          initialPreview={update?.preview}
          initialSourceURL={update?.url}
          onClose={() => {
            setImporting(false);
            setEditing(null);
            setUpdate(null);
          }}
          onImported={(ember) => {
            upsert(ember);
            void refresh();
            void refreshChecks();
          }}
          onDeleted={() => {
            void refresh();
          }}
        />
      )}
    </section>
  );
}
