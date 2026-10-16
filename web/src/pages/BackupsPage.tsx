import { useState } from "react";
import { listAllBackups } from "../api/backups";
import { bytes } from "../api/monitoring";
import { useCollection } from "../hooks/useCollection";
import { Table, TableRow } from "../components/table/Table";
import { StatusPill } from "../components/status/StatusPill";
import "./management.css";

export function BackupsPage() {
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
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Backups</h1>
          <p className="page-description">
            Backup archives across all servers, including local and S3 copies.
          </p>
        </div>
        <button
          className="button"
          disabled={backups.loading}
          onClick={() => {
            void backups.refresh();
          }}
        >
          Refresh
        </button>
      </header>
      {backups.error && backups.items.length > 0 && (
        <p className="error-message" role="alert">
          {backups.error}
        </p>
      )}
      <div className="list-toolbar">
        <label>
          Search
          <input
            type="search"
            placeholder="Search backups by archive or server…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>
      </div>
      <div className="management-content">
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
                  <td>{item.server_name}</td>
                  <td>{new Date(item.modified_at).toLocaleString()}</td>
                  <td>{bytes(item.size)}</td>
                  <td>
                    {[item.local ? "Local" : null, item.remote ? "S3" : null]
                      .filter(Boolean)
                      .join(" · ") || "—"}
                  </td>
                  <td>
                    <StatusPill status={item.state} />
                  </td>
                  <td className="table-actions">
                    {item.state !== "deleting" &&
                      (item.local || item.remote) && (
                        <a
                          className="button"
                          href={`/api/v1/servers/${encodeURIComponent(item.server_id)}/backups/${encodeURIComponent(item.name)}`}
                          download
                        >
                          Download
                        </a>
                      )}
                  </td>
                </TableRow>
              );
            })}
          </tbody>
        </Table>
      </div>
    </section>
  );
}
