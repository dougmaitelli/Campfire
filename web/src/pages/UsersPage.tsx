import { useState } from "react";
import { deleteUser, listRoles, listUsers, type User } from "../api/access";
import { useCollection } from "../hooks/useCollection";
import { useAuthentication } from "../hooks/useAuthentication";
import { UserDialog } from "../dialogs/UserDialog";
import "./management.css";

export function UsersPage() {
  const users = useCollection(listUsers);
  const roles = useCollection(listRoles);
  const auth = useAuthentication();
  const [editing, setEditing] = useState<User | null>(null);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function remove(user: User) {
    if (
      busy ||
      !window.confirm(
        `Delete "${user.username}"? Transfer their owned servers first. Their server memberships and sessions will be removed.`,
      )
    )
      return;
    setBusy(true);
    setError(null);
    try {
      await deleteUser(user.id);
      await users.refresh();
      if (user.id === auth.session?.user_id) await auth.reload();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to delete user.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <header className="page-header management-header">
        <div>
          <h1 id="page-title">Users</h1>
          <p className="page-description">
            Accounts, platform roles, and sign-in access.
          </p>
        </div>
        <div className="page-actions">
          <button
            className="button"
            onClick={() => {
              void users.refresh();
              void roles.refresh();
            }}
          >
            Refresh
          </button>
          <button
            className="button button-primary"
            disabled={roles.loading || !!roles.error}
            onClick={() => {
              setEditing(null);
              setOpen(true);
            }}
          >
            + Create user
          </button>
        </div>
      </header>
      {(error ?? users.error ?? roles.error) && (
        <p className="error-message" role="alert">
          {error ?? users.error ?? roles.error}
        </p>
      )}
      <div className="management-content">
        <div className="table-container">
          <table className="management-table">
            <caption className="visually-hidden">Panel users</caption>
            <thead>
              <tr>
                <th>User</th>
                <th>Platform role</th>
                <th>Status</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {users.items.map((user) => (
                <tr key={user.id}>
                  <td>
                    <strong>{user.name || user.username}</strong>
                    {user.name && (
                      <span className="row-description">{user.username}</span>
                    )}
                    {user.id === auth.session?.user_id && (
                      <span className="row-description">You</span>
                    )}
                  </td>
                  <td>{user.role.name}</td>
                  <td>{user.disabled ? "Disabled" : "Enabled"}</td>
                  <td>
                    <div className="row-actions">
                      <button
                        className="button"
                        disabled={busy}
                        onClick={() => {
                          setEditing(user);
                          setOpen(true);
                        }}
                      >
                        Edit
                      </button>
                      <button
                        className="button"
                        disabled={busy}
                        onClick={() => {
                          void remove(user);
                        }}
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      {users.loading && <p role="status">Loading users…</p>}
      <p className="field-help">
        At least one enabled administrator must remain. Accounts that own
        servers must transfer ownership before deletion.
      </p>
      {open && (
        <UserDialog
          user={editing}
          roles={roles.items}
          onClose={() => setOpen(false)}
          onSaved={(user) => {
            users.upsert(user);
            if (user.id === auth.session?.user_id) void auth.reload();
          }}
        />
      )}
    </section>
  );
}
