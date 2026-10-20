import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { ContentPanel } from "../components/layout/ContentPanel";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
import { useState } from "react";
import { deleteRole, listRoles, type Role } from "../api/access";
import { useCollection } from "../hooks/useCollection";
import { RoleDialog } from "../dialogs/RoleDialog";

import { Table, TableRow } from "../components/table/Table";
import { useResourceSelection } from "../routing/useResourceSelection";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";

const permissionLabels: Record<string, string> = {
  get "users.manage"() {
    return i18n.t("rolesPage.manageUsers");
  },
  get "roles.manage"() {
    return i18n.t("rolesPage.manageRoles");
  },
  get "sparks.manage"() {
    return i18n.t("rolesPage.manageSparks");
  },
  get "embers.manage"() {
    return i18n.t("rolesPage.manageEmbers");
  },
  get "servers.create"() {
    return i18n.t("rolesPage.createServers");
  },
  get "servers.manage_all"() {
    return i18n.t("rolesPage.manageAllServers");
  },
  get "server.view"() {
    return i18n.t("rolesPage.viewServer");
  },
  get "server.operate"() {
    return i18n.t("rolesPage.operateServer");
  },
  get "server.configure"() {
    return i18n.t("rolesPage.configureServer");
  },
  get "server.delete"() {
    return i18n.t("rolesPage.deleteServer");
  },
  get "server.share"() {
    return i18n.t("rolesPage.shareServer");
  },
};

export function RolesPage() {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const roles = useCollection(listRoles);
  const {
    selected: editing,
    select: setEditing,
    missing,
  } = useResourceSelection(roles.items);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  function edit(role: Role) {
    if (role.builtin || busy) return;
    setError(null);
    setEditing(role);
    setOpen(false);
  }
  async function remove(role: Role) {
    if (
      busy ||
      !window.confirm(
        i18n.t(
          "rolesPage.deleteRoleRolesAssignedToUsersOrServerMembersCannot",
          { roleName: role.name },
        ),
      )
    )
      return;
    setBusy(true);
    setError(null);
    try {
      await deleteRole(role.id);
      setOpen(false);
      setEditing(null);
      await roles.refresh();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("rolesPage.unableToDeleteRole"),
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.roles")}
        description={t("rolesPage.platformCapabilitiesAndServerScopedAccess")}
        actions={
          <>
            <Button
              onClick={() => {
                void roles.refresh();
              }}
            >
              {t("common.refresh")}
            </Button>
            <Button
              variant="primary"
              onClick={() => {
                setEditing(null);
                setOpen(true);
              }}
            >
              {t("rolesPage.createRole")}
            </Button>
          </>
        }
      />
      {(error ?? (roles.items.length ? roles.error : null)) && (
        <InlineMessage role="alert">{error ?? roles.error}</InlineMessage>
      )}
      <ContentPanel>
        <Table
          empty={roles.items.length === 0}
          loading={roles.loading}
          error={roles.error}
          resource={t("rolesPage.roles")}
          onRetry={() => {
            void roles.refresh();
          }}
        >
          <caption className="visually-hidden">
            {t("rolesPage.accessRoles")}
          </caption>
          <thead>
            <tr>
              <th>{t("common.role")}</th>
              <th>{t("rolesPage.scope")}</th>
              <th>{t("rolesPage.permissions")}</th>
            </tr>
          </thead>
          <tbody>
            {roles.items.map((role) => (
              <TableRow
                key={role.id}
                onOpen={!role.builtin && !busy ? () => edit(role) : undefined}
              >
                <td>
                  {role.builtin ? (
                    <ResourceLink href={links.role(role.id)}>
                      {role.name}
                    </ResourceLink>
                  ) : (
                    <button
                      type="button"
                      className="table-row-trigger"
                      disabled={busy}
                      aria-label={i18n.t("rolesPage.editRole", {
                        roleName: role.name,
                      })}
                      onClick={() => edit(role)}
                    >
                      {role.name}
                    </button>
                  )}
                  {role.builtin && (
                    <span className="row-description">
                      {i18n.t("rolesPage.builtIn")}
                    </span>
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
                    : i18n.t(
                        "rolesPage.accessThroughServerOwnershipOrMembership",
                      )}
                </td>
              </TableRow>
            ))}
          </tbody>
        </Table>
      </ContentPanel>
      {!roles.loading && !roles.error && missing && (
        <InlineMessage>
          {t("rolesPage.thisRoleIsUnavailableOrNoLongerExists")}
        </InlineMessage>
      )}
      {(open || editing) && (
        <RoleDialog
          key={editing?.id ?? "new"}
          role={editing}
          onClose={() => {
            setOpen(false);
            setEditing(null);
          }}
          onSaved={roles.upsert}
          onDelete={
            editing && !editing.builtin
              ? () => {
                  void remove(editing);
                }
              : undefined
          }
          deleting={busy}
          deleteError={error}
        />
      )}
    </section>
  );
}
