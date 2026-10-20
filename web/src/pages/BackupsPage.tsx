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
        title="Backups"
        description="Backup archives across all servers, including local and S3 copies."
        actions={
          <>
            <Button
              disabled={backups.loading}
              onClick={() => {
                void backups.refresh();
              }}
            >
              Refresh
            </Button>
          </>
        }
      />
      {backups.error && backups.items.length > 0 && (
        <InlineMessage role="alert">{backups.error}</InlineMessage>
      )}
      <ListToolbar>
        <SearchField
          label="Search"
          placeholder="Search backups by archive or server…"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </ListToolbar>
      <ContentPanel>
        <Table
          empty={filtered.length === 0}
          loading={backups.loading}
          error={backups.error}
          resource="backups"
          emptyMessage={query ? "No matching backups" : "No backups yet"}
          onRetry={() => {
            void backups.refresh();
          }}
        >
          <caption className="visually-hidden">All server backups</caption>
          <thead>
            <tr>
              <th>Archive</th>
              <th>Server</th>
              <th>Created</th>
              <th>Size</th>
              <th>Storage</th>
              <th>Status</th>
              <th className="table-actions">Actions</th>
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
                    {[item.local ? "Local" : null, item.remote ? "S3" : null]
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
                          aria-label={`Download ${item.name}`}
                          title="Download backup"
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
