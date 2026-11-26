import { RoleScope } from "../contracts/domains";
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
import { ServerPermissionTree } from "../components/permissions/ServerPermissionTree";
import { PlatformPermissionFields } from "../components/permissions/PlatformPermissionFields";
import {
  Permission,
  permissionCatalog,
  expandPermissions,
  type PermissionDefinition,
  type PermissionID,
} from "../api/permissions";
import { Fieldset } from "../components/forms/Fieldset";
import { RoleLimits } from "../config/policies";
import { serverPermissionPresets } from "../config/permissionPresets";
import { formatRoleName } from "../utils/formatRoleName";

const platformRolePermissions: PermissionID[] = [
  Permission.ServersCreate,
  Permission.ServersManageAll,
  Permission.UsersManage,
  Permission.RolesManage,
  Permission.SparksManage,
  Permission.EmbersManage,
];

export function RoleDrawer({
  role,
  existingRoles = [],
  onClose,
  onSaved,
  onDelete,
  deleting,
  deleteError,
}: {
  role: Role | null;
  existingRoles?: readonly Role[];
  onClose: () => void;
  onSaved: (role: Role) => void;
  onDelete?: (() => void) | undefined;
  deleting: boolean;
  deleteError: string | null;
}) {
  const { t } = useTranslation();
  const [customName, setCustomName] = useState<string | null>(
    role?.name ?? null,
  );
  const [scope, setScope] = useState<Role["scope"]>(
    role?.scope ?? RoleScope.Server,
  );
  const [permissions, setPermissions] = useState(
    role?.permissions ?? [Permission.ServerView],
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [catalog, setCatalog] = useState<PermissionDefinition[]>([]);
  const [catalogError, setCatalogError] = useState<string | null>(null);
  const name =
    customName ??
    formatRoleName({ scope, permissions, catalog, existingRoles });
  useEffect(() => {
    const controller = new AbortController();
    void permissionCatalog(controller.signal)
      .then((items) => {
        if (!controller.signal.aborted) {
          setCatalog(items);
          setPermissions((current) => expandPermissions(current, items));
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setCatalogError(
            cause instanceof Error
              ? cause.message
              : i18n.t("roleDrawer.unableToLoadPermissions"),
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
          : i18n.t("roleDrawer.unableToSaveRole"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  const platformChoices = platformRolePermissions.filter((id) =>
    role?.builtin
      ? role.permissions.includes(id)
      : id === Permission.ServersCreate,
  );
  const presets = serverPermissionPresets(catalog);
  return (
    <Drawer
      title={
        role?.builtin
          ? t("roleDrawer.viewRole")
          : role
            ? t("roleDrawer.editRole")
            : t("roleDrawer.createRole")
      }
      description={t(
        "roleDrawer.rolesGrantPanelCapabilitiesServerRolesApplyOnlyToAssigned",
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
                (scope === RoleScope.Server && !catalog.length)
              }
              onClick={() => {
                void submit();
              }}
              loading={busy}
              loadingLabel={t("roleDrawer.saving")}
            >
              {t("roleDrawer.saveRole")}
            </Button>
          )}
        </>
      }
    >
      <div className="panel-content form-stack">
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
            maxLength={RoleLimits.NameLength}
            onChange={(event) => setCustomName(event.target.value)}
          />
        </FormField>
        <FormField label={t("roleDrawer.scope")}>
          <Select
            id="role-scope"
            value={scope}
            disabled={role !== null}
            onValueChange={(selectedValue) => {
              setScope(selectedValue);
              setPermissions(
                selectedValue === RoleScope.Server
                  ? [Permission.ServerView]
                  : [],
              );
            }}
          >
            <option value={RoleScope.Server}>
              {t("common.serverAccessRole")}
            </option>
            <option value={RoleScope.Platform}>
              {t("common.platformRole")}
            </option>
          </Select>
        </FormField>
        <h3>{t("roleDrawer.permissions")}</h3>
        {scope === RoleScope.Server && !role?.builtin && (
          <FormField label={t("roleDrawer.applyPreset")}>
            <Select<(typeof presets)[number][0] | "">
              value=""
              disabled={busy || deleting || !catalog.length}
              onValueChange={(value) => {
                const preset = presets.find(([label]) => label === value);
                if (preset)
                  setPermissions(expandPermissions([...preset[1]], catalog));
              }}
            >
              <option value="">{t("roleDrawer.choosePreset")}</option>
              {presets.map(([label]) => (
                <option key={label} value={label}>
                  {t(`permissionPresets.${label}`)}
                </option>
              ))}
            </Select>
          </FormField>
        )}
        {scope === RoleScope.Server ? (
          <Fieldset
            className="form-stack form-stack-compact"
            disabled={busy || deleting || role?.builtin === true}
          >
            <legend className="visually-hidden">
              {t("roleDrawer.permissions")}
            </legend>
            <p className="field-help permission-tree-guidance">
              {t("roleDrawer.treeGuidance")}
            </p>
            <ServerPermissionTree
              catalog={catalog}
              selected={permissions}
              onChange={setPermissions}
              requireView
            />
          </Fieldset>
        ) : (
          <Fieldset
            className="permission-fields form-stack form-stack-compact"
            disabled={busy || deleting || role?.builtin === true}
          >
            <legend>{t("permissionGroups.Platform")}</legend>
            <PlatformPermissionFields
              choices={platformChoices}
              selected={permissions}
              onChange={setPermissions}
            />
          </Fieldset>
        )}
        {scope === RoleScope.Server && (
          <p className="field-help">{t("roleDrawer.permissionGuidance")}</p>
        )}
      </div>
    </Drawer>
  );
}
