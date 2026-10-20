import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
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
import { useResourceSelection } from "../routing/useResourceSelection";

export function UsersPage() {
  const { t } = useTranslation();
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
  const {
    selected: editing,
    select: setEditing,
    missing,
  } = useResourceSelection(users.items);
  const links = useResourceLinks();
  const [open, setOpen] = useState(false);
  function edit(user: User) {
    setEditing(user);
    setOpen(false);
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.users")}
        description={t("usersPage.accountsRolesAndSignInAccess")}
        actions={
          <>
            <Button
              onClick={() => {
                void users.refresh();
                void roles.refresh();
              }}
            >
              {t("common.refresh")}
            </Button>
            <Button
              variant="primary"
              disabled={roles.loading || !!roles.error}
              onClick={() => {
                setEditing(null);
                setOpen(true);
              }}
            >
              {t("usersPage.createUser")}
            </Button>
          </>
        }
      />
      {users.items.length > 0 && (users.error ?? roles.error) && (
        <InlineMessage role="alert">{users.error ?? roles.error}</InlineMessage>
      )}
      <ListToolbar>
        <SearchField
          label={t("common.search")}
          placeholder={t("usersPage.searchUsersByNameUsernameOrEmail")}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </ListToolbar>
      <ContentPanel>
        <Table
          empty={filtered.length === 0}
          loading={users.loading}
          error={users.error}
          resource={t("usersPage.users")}
          emptyMessage={
            query ? t("usersPage.noMatchingUsers") : t("usersPage.noUsersYet")
          }
          onRetry={() => {
            void users.refresh();
          }}
        >
          <caption className="visually-hidden">
            {t("usersPage.panelUsers")}
          </caption>
          <thead>
            <tr>
              <th>{t("common.user")}</th>
              <th>{t("common.role")}</th>
              <th>{t("common.status")}</th>
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
                    aria-label={i18n.t("usersPage.editUser", {
                      userNameUser: user.name || user.username,
                    })}
                  >
                    {user.name || user.username}
                    {user.id === auth.session?.user_id && " (you)"}
                  </button>
                  {user.name && (
                    <span className="row-description">{user.username}</span>
                  )}
                </td>
                <td>
                  <ResourceLink href={links.role(user.role_id)}>
                    {user.role.name}
                  </ResourceLink>
                </td>
                <td>
                  <StatusPill tone={user.disabled ? "warning" : "positive"}>
                    {user.disabled
                      ? i18n.t("common.disabled")
                      : i18n.t("common.enabled")}
                  </StatusPill>
                </td>
              </TableRow>
            ))}
          </tbody>
        </Table>
      </ContentPanel>
      <p className="field-help">
        {t("usersPage.atLeastOneEnabledAdministratorMustRemainAccountsThatOwn")}
      </p>
      {!users.loading && !users.error && missing && (
        <InlineMessage>
          {t("usersPage.thisUserIsUnavailableOrNoLongerExists")}
        </InlineMessage>
      )}
      {(open || editing) && (
        <UserDialog
          key={editing?.id ?? "new"}
          user={editing}
          roles={roles.items}
          onClose={() => {
            setOpen(false);
            setEditing(null);
          }}
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
