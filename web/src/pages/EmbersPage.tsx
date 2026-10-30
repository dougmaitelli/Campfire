import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { ContentPanel } from "../components/layout/ContentPanel";
import { Button } from "../components/forms/Button";
import { ListToolbar } from "../components/layout/ListToolbar";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
import { useState } from "react";
import {
  listEmbers,
  listEmberUpdates,
  checkEmberUpdate,
  downloadEmberUpdate,
  type Ember,
  type EmberPreview,
} from "../api/embers";
import { useCollection } from "../hooks/useCollection";
import { ImportEmberDrawer } from "../drawers/ImportEmberDrawer";

import { Table, TableRow } from "../components/table/Table";
import { StatusPill } from "../components/status/StatusPill";
import { PortPill } from "../components/network/PortPill";
import { GameArt } from "../components/media/GameArt";
import { EMPTY_VALUE } from "../utils/formatDisplayValue";
import { useResourceSelection } from "../routing/useResourceSelection";

export function EmbersPage() {
  const { t } = useTranslation();
  const { items, loading, error, refresh, upsert } = useCollection(listEmbers);
  const hasGameArt = items.some((ember) => ember.artUrl);
  const [importing, setImporting] = useState(false);
  const { selected: editing, select: setEditing } = useResourceSelection(
    items,
    !loading && !error,
  );
  const checks = useCollection(listEmberUpdates);
  const [busy, setBusy] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [update, setUpdate] = useState<{
    preview: EmberPreview;
    url: string;
  } | null>(null);
  const refreshChecks = checks.refresh;
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
        setActionError(
          i18n.t("embersPage.unableToCheckUpdatesFor", {
            failedJoin: failed.join(", "),
          }),
        );
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
        cause instanceof Error
          ? cause.message
          : i18n.t("embersPage.unableToDownloadUpdate"),
      );
    } finally {
      setBusy(null);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.embers")}
        description={t("embersPage.gameDefinitionsUsedToConfigureYourServers")}
        actions={
          <>
            <Button variant="primary" onClick={() => setImporting(true)}>
              {t("embersPage.importEmber")}
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
          loadingLabel={t("embersPage.checkingUpdates")}
        >
          {t("embersPage.checkUpdates")}
        </Button>
        <span className="field-help">
          {t(
            "embersPage.updatesAreCheckedDailyReviewUpdatesBeforeReplacingTheLocal",
          )}
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
          resource={t("common.embers")}
          emptyDescription={t(
            "embersPage.importAGameDefinitionBeforeCreatingServers",
          )}
          emptyAction={
            <Button variant="primary" onClick={() => setImporting(true)}>
              {t("embersPage.importEmber2")}
            </Button>
          }
          onRetry={() => {
            void refresh();
          }}
        >
          <caption className="visually-hidden">
            {t("embersPage.importedEmbers")}
          </caption>
          <thead>
            <tr>
              {hasGameArt && <th className="game-art-cell" />}
              <th
                scope="col"
                className={hasGameArt ? "game-art-name" : undefined}
              >
                {t("embersPage.gameDefinition")}
              </th>
              <th scope="col">{t("embersPage.images")}</th>
              <th scope="col">{t("embersPage.requiredPorts")}</th>
              <th scope="col">{t("embersPage.updates")}</th>
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
                  {hasGameArt && (
                    <td className="game-art-cell">
                      <GameArt src={ember.artUrl} />
                    </td>
                  )}
                  <td className={hasGameArt ? "game-art-name" : undefined}>
                    <button
                      type="button"
                      className="table-row-trigger"
                      aria-label={i18n.t("embersPage.editEmber", {
                        emberName: ember.name,
                      })}
                      onClick={() => setEditing(ember)}
                    >
                      {ember.name}
                    </button>
                    {ember.origin && (
                      <span className="row-description">
                        <Trans
                          i18nKey="embersPage.importedFrom"
                          t={i18n.t}
                          components={{
                            emberOriginFormat: <>{ember.origin.format}</>,
                          }}
                        />
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
                            description={port.description}
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
                          loadingLabel={i18n.t("embersPage.downloading")}
                        >
                          {i18n.t("common.update")}
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
                              ? i18n.t("embersPage.checkFailed")
                              : status
                                ? i18n.t("embersPage.upToDate")
                                : i18n.t("embersPage.notCheckedYet")}
                          </StatusPill>
                          {status?.error && (
                            <span className="row-description">
                              {status.error}
                            </span>
                          )}
                        </>
                      )
                    ) : (
                      i18n.t("embersPage.noUpdateURL")
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
          ? t("common.loading")
          : t("embersPage.count", { count: items.length })}
      </p>
      {(importing || editing) && (
        <ImportEmberDrawer
          key={editing?.id ?? "new"}
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
