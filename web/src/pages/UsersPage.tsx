import { ContentPanel } from "../components/layout/ContentPanel";
import { SearchField } from "../components/forms/SearchField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { ListToolbar } from "../components/layout/ListToolbar";
import { PageHeader } from "../components/layout/PageHeader";
import { useState } from "react";
import { listRoles, listUsers, type User } from "../api/access";
import { useCollection } from "../hooks/useCollection";
import { useAuthentication } from "../hooks/useAuthentication";
import { UserDialog } from "../dialogs/UserDialog";
import { StatusPill } from "../components/status/StatusPill";

import { Table, TableRow } from "../components/table/Table";

export function UsersPage() {
  const users = useCollection(listUsers);
  const roles = useCollection(listRoles);
  const auth = useAuthentication();
  const [search, setSearch] = useState("");
  const query = search.trim().toLowerCase();
  const filtered = users.items.filter((user) =>
    [user.name, user.username, user.email].some((value) =>
      value.toLowerCase().includes(query),
    ),
  );
  const [editing, setEditing] = useState<User | null>(null);
  const [open, setOpen] = useState(false);
  function edit(user: User) {
    setEditing(user);
    setOpen(true);
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title="Users"
        description="Accounts, roles, and sign-in access."
        actions={
          <>
            <Button
              onClick={() => {
                void users.refresh();
                void roles.refresh();
              }}
            >
              Refresh
            </Button>
            <Button
              variant="primary"
              disabled={roles.loading || !!roles.error}
              onClick={() => {
                setEditing(null);
                setOpen(true);
              }}
            >
              + Create user
            </Button>
          </>
        }
      />
      {users.items.length > 0 && (users.error ?? roles.error) && (
        <InlineMessage role="alert">{users.error ?? roles.error}</InlineMessage>
      )}
      <ListToolbar>
        <SearchField
          label="Search"
          placeholder="Search users by name, username, or email…"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </ListToolbar>
      <ContentPanel>
        <Table
          empty={filtered.length === 0}
          loading={users.loading}
          error={users.error}
          resource="users"
          emptyMessage={query ? "No matching users" : "No users yet"}
          onRetry={() => {
            void users.refresh();
          }}
        >
          <caption className="visually-hidden">Panel users</caption>
          <thead>
            <tr>
              <th>User</th>
              <th>Role</th>
              <th>Status</th>
            </tr>
          </thead>
          <tbody>
            {filtered.map((user) => (
              <TableRow key={user.id} onOpen={() => edit(user)}>
                <td>
                  <button
                    type="button"
                    className="table-row-trigger"
                    onClick={() => edit(user)}
                    aria-label={`Edit user ${user.name || user.username}`}
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
                  <StatusPill tone={user.disabled ? "warning" : "positive"}>
                    {user.disabled ? "Disabled" : "Enabled"}
                  </StatusPill>
                </td>
              </TableRow>
            ))}
          </tbody>
        </Table>
      </ContentPanel>
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
          onDeleted={() => {
            void users.refresh();
            if (editing?.id === auth.session?.user_id) void auth.reload();
          }}
        />
      )}
    </section>
  );
}
