import { useState } from "react";
import { deleteRole, listRoles, type Role } from "../api/access";
import { useCollection } from "../hooks/useCollection";
import { RoleDialog } from "../dialogs/RoleDialog";
import "./management.css";
import { Table } from "../components/table/Table";

const permissionLabels: Record<string, string> = {
  "users.manage": "Manage users",
  "roles.manage": "Manage roles",
  "sparks.manage": "Manage Sparks",
  "embers.manage": "Manage Embers",
  "servers.create": "Create servers",
  "servers.manage_all": "Manage all servers",
  "server.view": "View server",
  "server.operate": "Operate server",
  "server.configure": "Configure server",
  "server.delete": "Delete server",
  "server.share": "Share server",
};

export function RolesPage() {
  const roles = useCollection(listRoles);
  const [editing, setEditing] = useState<Role | null>(null);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  async function remove(role: Role) {
    if (
      busy ||
      !window.confirm(
        `Delete role "${role.name}"? Roles assigned to users or server members cannot be deleted.`,
      )
    )
      return;
    setBusy(true);
    setError(null);
    try {
      await deleteRole(role.id);
      await roles.refresh();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to delete role.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Roles</h1>
          <p className="page-description">
            Platform capabilities and server-scoped access.
          </p>
        </div>
        <div className="page-actions">
          <button
            className="button"
            onClick={() => {
              void roles.refresh();
            }}
          >
            Refresh
          </button>
          <button
            className="button button-primary"
            onClick={() => {
              setEditing(null);
              setOpen(true);
            }}
          >
            + Create role
          </button>
        </div>
      </header>
      {(error ?? roles.error) && (
        <p className="error-message" role="alert">
          {error ?? roles.error}
        </p>
      )}
      <div className="management-content">
        <Table>
          <caption className="visually-hidden">Access roles</caption>
          <thead>
            <tr>
              <th>Role</th>
              <th>Scope</th>
              <th>Permissions</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody>
            {roles.items.map((role) => (
              <tr key={role.id}>
                <td>
                  <strong>{role.name}</strong>
                  {role.builtin && (
                    <span className="row-description">Built-in</span>
                  )}
                </td>
                <td>{role.scope}</td>
                <td>
                  {role.permissions.length
                    ? role.permissions
                        .map(
                          (permission) =>
                            permissionLabels[permission] ?? permission,
                        )
                        .join(", ")
                    : "Access through server ownership or membership"}
                </td>
                <td>
                  {!role.builtin && (
                    <div className="row-actions">
                      <button
                        className="button"
                        disabled={busy}
                        onClick={() => {
                          setEditing(role);
                          setOpen(true);
                        }}
                      >
                        Edit
                      </button>
                      <button
                        className="button"
                        disabled={busy}
                        onClick={() => {
                          void remove(role);
                        }}
                      >
                        Delete
                      </button>
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
      </div>
      {roles.loading && <p role="status">Loading roles…</p>}
      {open && (
        <RoleDialog
          role={editing}
          onClose={() => setOpen(false)}
          onSaved={roles.upsert}
        />
      )}
    </section>
  );
}
