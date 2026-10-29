import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Select } from "../components/forms/Select";
import { useEffect, useRef, useState } from "react";
import { saveRole, type Role } from "../api/access";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { CheckboxField } from "../components/forms/CheckboxField";
import {
  Permission,
  permissionCatalog,
  expandPermissions,
  togglePermission,
  type PermissionDefinition,
} from "../api/permissions";
import { Fieldset } from "../components/forms/Fieldset";
import "./role-dialog.css";

const presets = [
  ["Observer", Permission.ServerView],
  ["Log viewer", Permission.ServerConsoleRead],
  ["Power operator", Permission.ServerOperate],
  ["Console operator", Permission.ServerConsoleWrite],
  ["File maintainer", Permission.ServerFilesWrite],
  ["Backup creator", Permission.ServerBackupsCreate],
  ["Backup restorer", Permission.ServerBackupsRestore],
] as const;

export function RoleDialog({
  role,
  onClose,
  onSaved,
  onDelete,
  deleting,
  deleteError,
}: {
  role: Role | null;
  onClose: () => void;
  onSaved: (role: Role) => void;
  onDelete?: (() => void) | undefined;
  deleting: boolean;
  deleteError: string | null;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(role?.name ?? "");
  const [scope, setScope] = useState<Role["scope"]>(role?.scope ?? "server");
  const [permissions, setPermissions] = useState(
    role?.permissions ?? [Permission.ServerView],
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [catalog, setCatalog] = useState<PermissionDefinition[]>([]);
  const [catalogError, setCatalogError] = useState<string | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    void permissionCatalog(controller.signal)
      .then((items) => {
        if (!controller.signal.aborted) setCatalog(items);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setCatalogError(
            cause instanceof Error
              ? cause.message
              : i18n.t("roleDialog.unableToLoadPermissions"),
          );
      });
    return () => controller.abort();
  }, []);
  const pending = useRef(false);
  async function submit() {
    if (pending.current || role?.builtin) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      onSaved(await saveRole({ name, scope, permissions }, role?.id));
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("roleDialog.unableToSaveRole"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  const platformChoices: PermissionDefinition[] = (
    [
      [
        Permission.ServersCreate,
        t("roleDialog.createServersOnSparksApprovedByAnAdministrator"),
      ],
      [Permission.ServersManageAll, t("rolesPage.manageAllServers")],
      [Permission.UsersManage, t("rolesPage.manageUsers")],
      [Permission.RolesManage, t("rolesPage.manageRoles")],
      [Permission.SparksManage, t("rolesPage.manageSparks")],
      [Permission.EmbersManage, t("rolesPage.manageEmbers")],
    ] as const
  ).map(([id, label]) => ({ id, group: "Platform", label, requires: [] }));
  const choices =
    scope === "server"
      ? catalog
      : platformChoices.filter((item) =>
          role?.builtin
            ? role.permissions.includes(item.id)
            : item.id === Permission.ServersCreate,
        );
  const groups = [...new Set(choices.map((item) => item.group))];
  function toggle(id: string, checked: boolean) {
    const next = togglePermission(permissions, id, checked, catalog);
    const removed = permissions.filter(
      (item) => item !== id && !next.includes(item),
    );
    if (
      removed.length &&
      !window.confirm(
        t("roleDialog.removeDependencies", {
          permissions: removed
            .map(
              (item) =>
                catalog.find((entry) => entry.id === item)?.label ?? item,
            )
            .join(", "),
        }),
      )
    )
      return;
    setPermissions(next);
  }
  return (
    <Drawer
      title={
        role?.builtin
          ? t("roleDialog.viewRole")
          : role
            ? t("roleDialog.editRole")
            : t("roleDialog.createRole")
      }
      description={t(
        "roleDialog.rolesGrantPanelCapabilitiesServerRolesApplyOnlyToAssigned",
      )}
      busy={busy || deleting}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy || deleting} onClick={onClose}>
          {t("common.cancel")}
        </Button>
      }
      footerRight={
        <>
          <DrawerDeleteButton
            onDelete={onDelete}
            disabled={busy}
            deleting={deleting}
          />
          {!role?.builtin && (
            <Button
              variant="primary"
              disabled={
                busy ||
                deleting ||
                !name.trim() ||
                (scope === "server" && !catalog.length)
              }
              onClick={() => {
                void submit();
              }}
              loading={busy}
              loadingLabel={t("roleDialog.saving")}
            >
              {t("roleDialog.saveRole")}
            </Button>
          )}
        </>
      }
    >
      <div className="drawer-content">
        {deleteError && (
          <InlineMessage role="alert">{deleteError}</InlineMessage>
        )}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {catalogError && (
          <InlineMessage role="alert">{catalogError}</InlineMessage>
        )}
        <FormField label={t("common.name")}>
          <TextInput
            id="role-name"
            readOnly={role?.builtin}
            value={name}
            maxLength={64}
            onChange={(event) => setName(event.target.value)}
          />
        </FormField>
        <FormField label={t("roleDialog.scope")}>
          <Select
            id="role-scope"
            value={scope}
            disabled={role !== null}
            onValueChange={(selectedValue) => {
              const next = selectedValue === "platform" ? "platform" : "server";
              setScope(next);
              setPermissions(next === "server" ? [Permission.ServerView] : []);
            }}
          >
            <option value="server">{t("common.server")}</option>
            <option value="platform">{t("roleDialog.platform")}</option>
          </Select>
        </FormField>
        <h3>{t("roleDialog.permissions")}</h3>
        {scope === "server" && !role?.builtin && (
          <FormField label={t("roleDialog.applyPreset")}>
            <Select
              value=""
              disabled={busy || deleting || !catalog.length}
              onValueChange={(value) => {
                if (presets.some(([label]) => label === value)) {
                  const preset = presets.find(([label]) => label === value);
                  if (preset)
                    setPermissions(expandPermissions([preset[1]], catalog));
                }
              }}
            >
              <option value="">{t("roleDialog.choosePreset")}</option>
              {presets.map(([label]) => (
                <option key={label} value={label}>
                  {t(`permissionPresets.${label}`)}
                </option>
              ))}
            </Select>
          </FormField>
        )}
        {groups.map((group) => (
          <Fieldset
            className="role-permissions"
            key={group}
            disabled={busy || deleting || role?.builtin === true}
          >
            <legend>
              {t(`permissionGroups.${group}`, { defaultValue: group })}
            </legend>
            {choices
              .filter((item) => item.group === group)
              .map((item) => (
                <CheckboxField
                  key={item.id}
                  checked={permissions.includes(item.id)}
                  disabled={item.id === Permission.ServerView}
                  onChange={(event) => toggle(item.id, event.target.checked)}
                >
                  {t(`permissionLabels.${item.id.replaceAll(".", "_")}`, {
                    defaultValue: item.label,
                  })}
                  {item.requires.length > 0 && (
                    <span className="field-help">
                      {t("roleDialog.requires", {
                        permissions: item.requires
                          .map((id) =>
                            t(`permissionLabels.${id.replaceAll(".", "_")}`, {
                              defaultValue:
                                catalog.find((entry) => entry.id === id)
                                  ?.label ?? id,
                            }),
                          )
                          .join("; "),
                      })}
                    </span>
                  )}
                </CheckboxField>
              ))}
          </Fieldset>
        ))}
        {scope === "server" && (
          <p className="field-help">{t("roleDialog.permissionGuidance")}</p>
        )}
        <p className="field-help">
          {t(
            "roleDialog.serverPermissionsNeverGrantAccessToUnassignedServersSparkUser",
          )}
        </p>
      </div>
    </Drawer>
  );
}
