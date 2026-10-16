import { useState } from "react";
import { deleteUser, listRoles, listUsers, type User } from "../api/access";
import { useCollection } from "../hooks/useCollection";
import { useAuthentication } from "../hooks/useAuthentication";
import { UserDialog } from "../dialogs/UserDialog";
import { UserDetailsDialog } from "../dialogs/UserDetailsDialog";
import { StatusPill } from "../components/status/StatusPill";
import "./management.css";
import { Table, TableRow } from "../components/table/Table";

export function UsersPage() {
  const users = useCollection(listUsers);
  const roles = useCollection(listRoles);
  const auth = useAuthentication();
  const [search, setSearch] = useState("");
  const query = search.trim().toLowerCase();
  const filtered = users.items.filter((user) =>
    [user.name, user.username].some((value) =>
      value.toLowerCase().includes(query),
    ),
  );
  const [editing, setEditing] = useState<User | null>(null);
  const [details, setDetails] = useState<User | null>(null);
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
      setDetails(null);
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
      <div className="list-toolbar">
        <label>
          Search
          <input
            type="search"
            placeholder="Search users by name or username…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>
      </div>
      <div className="management-content">
        <Table>
          <caption className="visually-hidden">Panel users</caption>
          <thead>
            <tr>
              <th>User</th>
              <th>Platform role</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((user) => (
              <TableRow key={user.id} onOpen={() => setDetails(user)}>
                <td>
                  <button
                    type="button"
                    className="table-row-trigger"
                    onClick={() => setDetails(user)}
                    aria-label={`View details for ${user.name || user.username}`}
                  >
                    {user.name || user.username}
                    {user.id === auth.session?.user_id && " (you)"}
                  </button>
                  {user.name && (
                    <span className="row-description">{user.username}</span>
                  )}
                </td>
                <td>{user.role.name}</td>
                <td>
                  <StatusPill tone={user.disabled ? "neutral" : "positive"}>
                    {user.disabled ? "Disabled" : "Enabled"}
                  </StatusPill>
                </td>
              </TableRow>
            ))}
            {!users.loading && !users.error && filtered.length === 0 && (
              <tr>
                <td colSpan={3}>
                  {query ? "No users match your search." : "No users yet."}
                </td>
              </tr>
            )}
          </tbody>
        </Table>
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
      {details && (
        <UserDetailsDialog
          user={details}
          onDelete={() => {
            void remove(details);
          }}
          busy={busy}
          error={error}
          onClose={() => setDetails(null)}
          onEdit={() => {
            setDetails(null);
            setEditing(details);
            setOpen(true);
          }}
        />
      )}
    </section>
  );
}
