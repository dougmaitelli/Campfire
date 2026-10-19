import { ContentPanel } from "../components/layout/ContentPanel";
import { Button } from "../components/forms/Button";
import { ListToolbar } from "../components/layout/ListToolbar";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
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

import { Table, TableRow } from "../components/table/Table";
import { StatusPill } from "../components/status/StatusPill";
import { PortPill } from "../components/network/PortPill";
import { EMPTY_VALUE } from "../utils/formatDisplayValue";

export function EmbersPage() {
  const { items, loading, error, refresh, upsert } = useCollection(listEmbers);
  const [importing, setImporting] = useState(false);
  const [editing, setEditing] = useState<Ember | null>(null);
  const checks = useCollection(listEmberUpdates);
  const [busy, setBusy] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
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
  async function checkAll() {
    if (busy || checking) return;
    setChecking(true);
    setActionError(null);
    const failed: string[] = [];
    try {
      for (const ember of items) {
        if (!(ember.updateUrl ?? ember.origin?.update_url)) continue;
        try {
          checks.upsert(await checkEmberUpdate(ember.id));
        } catch {
          failed.push(ember.name);
        }
      }
      if (failed.length > 0) {
        setActionError(`Unable to check updates for: ${failed.join(", ")}.`);
      }
    } finally {
      setChecking(false);
    }
  }
  async function download(ember: Ember) {
    if (busy || checking) return;
    setBusy(ember.id);
    setActionError(null);
    try {
      const preview = await downloadEmberUpdate(ember.id);
      setEditing(ember);
      setUpdate({
        preview,
        url: ember.updateUrl ?? ember.origin?.update_url ?? "",
      });
      setImporting(true);
      await refreshChecks();
    } catch (cause) {
      setActionError(
        cause instanceof Error ? cause.message : "Unable to download update.",
      );
    } finally {
      setBusy(null);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title="Embers"
        description="Game definitions used to configure your servers."
        actions={
          <>
            <Button
              disabled={loading}
              onClick={() => {
                void refresh();
                void refreshChecks();
              }}
              loading={loading}
              loadingLabel={"Refreshing…"}
            >
              {"Refresh"}
            </Button>
            <Button variant="primary" onClick={() => setImporting(true)}>
              + Import Ember
            </Button>
          </>
        }
      />
      <ListToolbar>
        <Button
          disabled={
            loading ||
            checking ||
            busy !== null ||
            !items.some((ember) => ember.updateUrl ?? ember.origin?.update_url)
          }
          onClick={() => {
            void checkAll();
          }}
          loading={checking}
          loadingLabel={"Checking updates…"}
        >
          {"Check updates"}
        </Button>
        <span className="field-help">
          Updates are checked daily. Review updates before replacing the local
          Ember.
        </span>
      </ListToolbar>
      {error && items.length > 0 && (
        <InlineMessage role="alert">{error}</InlineMessage>
      )}
      {(actionError ?? checks.error) && (
        <InlineMessage role="alert">
          {actionError ?? checks.error}
        </InlineMessage>
      )}
      <ContentPanel aria-busy={loading}>
        <Table
          empty={items.length === 0}
          loading={loading}
          error={error}
          resource="Embers"
          emptyDescription="Import a game definition before creating servers."
          emptyAction={
            <Button variant="primary" onClick={() => setImporting(true)}>
              Import Ember
            </Button>
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
              <th scope="col">Updates</th>
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
                  <td>{Object.keys(ember.images).join(", ")}</td>
                  <td>
                    {ember.required_ports?.length ? (
                      <div className="pill-group">
                        {ember.required_ports.map((port) => (
                          <PortPill
                            key={`${port.container}/${port.protocol}`}
                            port={port.container}
                            protocol={port.protocol}
                          />
                        ))}
                      </div>
                    ) : (
                      EMPTY_VALUE
                    )}
                  </td>
                  <td>
                    {updateURL ? (
                      status?.available && !status.error ? (
                        <Button
                          variant="primary"
                          disabled={busy !== null || checking}
                          onClick={() => {
                            void download(ember);
                          }}
                          loading={busy === ember.id}
                          loadingLabel={"Downloading…"}
                        >
                          {"Update"}
                        </Button>
                      ) : (
                        <>
                          <StatusPill
                            tone={
                              status?.error
                                ? "negative"
                                : status
                                  ? "positive"
                                  : "neutral"
                            }
                          >
                            {status?.error
                              ? "Check failed"
                              : status
                                ? "Up to date"
                                : "Not checked yet"}
                          </StatusPill>
                          {status?.error && (
                            <span className="row-description">
                              {status.error}
                            </span>
                          )}
                        </>
                      )
                    ) : (
                      "No update URL"
                    )}
                  </td>
                </TableRow>
              );
            })}
          </tbody>
        </Table>
      </ContentPanel>
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
