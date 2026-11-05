import { ServerTab, BackupState as BackupStatus } from "../contracts/domains";
import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { SearchField } from "../components/forms/SearchField";
import { ButtonLink } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { ListToolbar } from "../components/layout/ListToolbar";
import { PageHeader } from "../components/layout/PageHeader";
import { useState } from "react";
import { listAllBackups } from "../api/backups";
import { formatBytes } from "../utils/formatBytes";
import { useCollection } from "../hooks/useCollection";
import { Table, TableRow } from "../components/table/Table";
import { BackupState } from "../components/servers/BackupState";
import { NavigationIcon } from "../components/navigation/NavigationIcon";
import { BackupName } from "../components/servers/BackupName";
import { BackupMetadataActions } from "../components/servers/BackupMetadataActions";
import { backupDisplayName } from "../utils/backupDisplayName";
import { ActionGroup } from "../components/layout/ActionGroup";
import "./backups.css";

export function BackupsPage() {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const backups = useCollection(listAllBackups);
  const [search, setSearch] = useState("");
  const [actionError, setActionError] = useState<string | null>(null);
  const query = search.trim().toLowerCase();
  const filtered = backups.items.filter((item) =>
    [item.name, item.display_name, item.server_name, item.server_id].some(
      (value) => value.toLowerCase().includes(query),
    ),
  );
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.backups")}
        description={t("backupsPage.backupArchivesDescription")}
      />
      {backups.error && backups.items.length > 0 && (
        <InlineMessage role="alert">{backups.error}</InlineMessage>
      )}
      {actionError && <InlineMessage role="alert">{actionError}</InlineMessage>}
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
              <th className="table-mobile-hidden">
                {t("backupsPage.created")}
              </th>
              <th className="table-mobile-hidden">{t("backupsPage.size")}</th>
              <th className="table-mobile-hidden">
                {t("backupsPage.storage")}
              </th>
              <th className="table-mobile-hidden">{t("common.status")}</th>
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
                    <BackupName backup={item}>
                      <a className="table-row-trigger" href={route}>
                        {backupDisplayName(item)}
                      </a>
                    </BackupName>
                    {item.error && (
                      <span className="row-description">{item.error}</span>
                    )}
                  </td>
                  <td>
                    <ResourceLink
                      href={links.server(item.server_id, ServerTab.Backups)}
                    >
                      {item.server_name}
                    </ResourceLink>
                    <span className="backup-catalog-mobile-status">
                      <BackupState state={item.state} />
                    </span>
                  </td>
                  <td className="table-mobile-hidden">
                    {new Date(item.modified_at).toLocaleString()}
                  </td>
                  <td className="table-mobile-hidden">
                    {formatBytes(item.size)}
                  </td>
                  <td className="table-mobile-hidden">
                    {[
                      item.local ? i18n.t("backupsPage.local") : null,
                      item.remote ? "S3" : null,
                    ]
                      .filter(Boolean)
                      .join(" · ") || "—"}
                  </td>
                  <td className="table-mobile-hidden">
                    <BackupState state={item.state} />
                  </td>
                  <td className="table-actions">
                    <ActionGroup>
                      {item.can_download &&
                        item.state !== BackupStatus.Deleting &&
                        (item.local || item.remote) && (
                          <ButtonLink
                            href={`/api/v1/servers/${encodeURIComponent(item.server_id)}/backups/${encodeURIComponent(item.name)}`}
                            download
                            iconOnly
                            aria-label={i18n.t("backupsPage.download", {
                              itemName: backupDisplayName(item),
                            })}
                            title={i18n.t("backupsPage.downloadBackup")}
                          >
                            <NavigationIcon name="download" />
                          </ButtonLink>
                        )}
                      {item.can_edit && (
                        <BackupMetadataActions
                          serverID={item.server_id}
                          backup={item}
                          disabled={item.state === BackupStatus.Deleting}
                          onSaved={(updated) =>
                            backups.upsert({ ...item, ...updated })
                          }
                          onError={setActionError}
                        />
                      )}
                    </ActionGroup>
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
