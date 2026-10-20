import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { SearchField } from "../components/forms/SearchField";
import { Button, ButtonLink } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { ListToolbar } from "../components/layout/ListToolbar";
import { PageHeader } from "../components/layout/PageHeader";
import { useState } from "react";
import { listAllBackups } from "../api/backups";
import { bytes } from "../api/monitoring";
import { useCollection } from "../hooks/useCollection";
import { Table, TableRow } from "../components/table/Table";
import { BackupState } from "../components/servers/BackupState";
import { NavigationIcon } from "../components/navigation/NavigationIcon";

export function BackupsPage() {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const backups = useCollection(listAllBackups);
  const [search, setSearch] = useState("");
  const query = search.trim().toLowerCase();
  const filtered = backups.items.filter((item) =>
    [item.name, item.server_name, item.server_id].some((value) =>
      value.toLowerCase().includes(query),
    ),
  );
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.backups")}
        description={t(
          "backupsPage.backupArchivesAcrossAllServersIncludingLocalAndS3Copies",
        )}
        actions={
          <>
            <Button
              disabled={backups.loading}
              onClick={() => {
                void backups.refresh();
              }}
            >
              {t("common.refresh")}
            </Button>
          </>
        }
      />
      {backups.error && backups.items.length > 0 && (
        <InlineMessage role="alert">{backups.error}</InlineMessage>
      )}
      <ListToolbar>
        <SearchField
          label={t("common.search")}
          placeholder={t("backupsPage.searchBackupsByArchiveOrServer")}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </ListToolbar>
      <ContentPanel>
        <Table
          empty={filtered.length === 0}
          loading={backups.loading}
          error={backups.error}
          resource={t("backupsPage.backups")}
          emptyMessage={
            query
              ? t("backupsPage.noMatchingBackups")
              : t("backupsPage.noBackupsYet")
          }
          onRetry={() => {
            void backups.refresh();
          }}
        >
          <caption className="visually-hidden">
            {t("backupsPage.allServerBackups")}
          </caption>
          <thead>
            <tr>
              <th>{t("backupsPage.archive")}</th>
              <th>{t("common.server")}</th>
              <th>{t("backupsPage.created")}</th>
              <th>{t("backupsPage.size")}</th>
              <th>{t("backupsPage.storage")}</th>
              <th>{t("common.status")}</th>
              <th className="table-actions">
                <span className="visually-hidden">{t("common.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((item) => {
              const route = `#servers/${encodeURIComponent(item.server_id)}/backups`;
              return (
                <TableRow
                  key={item.id}
                  onOpen={() => {
                    window.location.hash = route;
                  }}
                >
                  <td>
                    <a className="table-row-trigger" href={route}>
                      {item.name}
                    </a>
                    {item.error && (
                      <span className="row-description">{item.error}</span>
                    )}
                  </td>
                  <td>
                    <ResourceLink
                      href={links.server(item.server_id, "backups")}
                    >
                      {item.server_name}
                    </ResourceLink>
                  </td>
                  <td>{new Date(item.modified_at).toLocaleString()}</td>
                  <td>{bytes(item.size)}</td>
                  <td>
                    {[
                      item.local ? i18n.t("backupsPage.local") : null,
                      item.remote ? "S3" : null,
                    ]
                      .filter(Boolean)
                      .join(" · ") || "—"}
                  </td>
                  <td>
                    <BackupState state={item.state} />
                  </td>
                  <td className="table-actions">
                    {item.state !== "deleting" &&
                      (item.local || item.remote) && (
                        <ButtonLink
                          href={`/api/v1/servers/${encodeURIComponent(item.server_id)}/backups/${encodeURIComponent(item.name)}`}
                          download
                          iconOnly
                          aria-label={i18n.t("backupsPage.download", {
                            itemName: item.name,
                          })}
                          title={i18n.t("backupsPage.downloadBackup")}
                        >
                          <NavigationIcon name="download" />
                        </ButtonLink>
                      )}
                  </td>
                </TableRow>
              );
            })}
          </tbody>
        </Table>
      </ContentPanel>
    </section>
  );
}
