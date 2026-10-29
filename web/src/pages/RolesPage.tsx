import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { ContentPanel } from "../components/layout/ContentPanel";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
import { useEffect, useState } from "react";
import {
  Permission,
  permissionCatalog,
  type PermissionDefinition,
} from "../api/permissions";
import { deleteRole, listRoles, type Role } from "../api/access";
import { useCollection } from "../hooks/useCollection";
import { RoleDialog } from "../dialogs/RoleDialog";

import { Table, TableRow } from "../components/table/Table";
import { useResourceSelection } from "../routing/useResourceSelection";

const permissionLabels: Record<string, string> = {
  get [Permission.UsersManage]() {
    return i18n.t("rolesPage.manageUsers");
  },
  get [Permission.RolesManage]() {
    return i18n.t("rolesPage.manageRoles");
  },
  get [Permission.SparksManage]() {
    return i18n.t("rolesPage.manageSparks");
  },
  get [Permission.EmbersManage]() {
    return i18n.t("rolesPage.manageEmbers");
  },
  get [Permission.ServersCreate]() {
    return i18n.t("rolesPage.createServers");
  },
  get [Permission.ServersManageAll]() {
    return i18n.t("rolesPage.manageAllServers");
  },
  get [Permission.ServerView]() {
    return i18n.t("rolesPage.viewServer");
  },
  get [Permission.ServerOperate]() {
    return i18n.t("rolesPage.operateServer");
  },
  get [Permission.ServerConfigure]() {
    return i18n.t("rolesPage.configureServer");
  },
  get [Permission.ServerDelete]() {
    return i18n.t("rolesPage.deleteServer");
  },
};

export function RolesPage() {
  const { t } = useTranslation();
  const roles = useCollection(listRoles);
  const { selected: editing, select: setEditing } = useResourceSelection(
    roles.items,
    !roles.loading && !roles.error,
  );
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [catalog, setCatalog] = useState<PermissionDefinition[]>([]);
  const [catalogLoading, setCatalogLoading] = useState(true);
  useEffect(() => {
    const controller = new AbortController();
    void permissionCatalog(controller.signal)
      .then((items) => {
        if (!controller.signal.aborted) setCatalog(items);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : t("roleDialog.unableToLoadPermissions"),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setCatalogLoading(false);
      });
    return () => controller.abort();
  }, [t]);
  function edit(role: Role) {
    if (busy) return;
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
      <ContentPanel aria-busy={roles.loading || catalogLoading}>
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
                onOpen={!busy ? () => edit(role) : undefined}
              >
                <td>
                  {role.builtin ? (
                    <button
                      type="button"
                      className="table-row-trigger"
                      disabled={busy}
                      onClick={() => edit(role)}
                    >
                      {role.name}
                    </button>
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
                  {role.scope === "server" && role.permissions.length > 6
                    ? t("rolesPage.permissionSummary", {
                        count: role.permissions.length,
                        groups: [
                          ...new Set(
                            catalog
                              .filter((item) =>
                                role.permissions.includes(item.id),
                              )
                              .map((item) =>
                                t(`permissionGroups.${item.group}`, {
                                  defaultValue: item.group,
                                }),
                              ),
                          ),
                        ].join(", "),
                      })
                    : role.permissions.length
                      ? role.permissions
                          .map((permission) =>
                            t(
                              `permissionLabels.${permission.replaceAll(".", "_")}`,
                              {
                                defaultValue:
                                  catalog.find((item) => item.id === permission)
                                    ?.label ??
                                  permissionLabels[permission] ??
                                  permission,
                              },
                            ),
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
