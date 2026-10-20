import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Select } from "../components/forms/Select";
import { useRef, useState } from "react";
import { saveRole, type Role } from "../api/access";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { CheckboxField } from "../components/forms/CheckboxField";

const serverPermissions = [
  ["server.view", i18n.t("roleDialog.viewTheServerAndRuntimeStatus")],
  [
    "server.operate",
    i18n.t("roleDialog.lifecycleControlsConsoleFilesAndBackups"),
  ],
  [
    "server.configure",
    i18n.t("roleDialog.editServerConfigurationAndResourceReservations"),
  ],
  ["server.delete", i18n.t("roleDialog.deleteTheServerAndGameData")],
  ["server.share", i18n.t("roleDialog.shareAccessWithOperators")],
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
    role?.permissions ?? ["server.view"],
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
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
  const choices =
    scope === "server"
      ? serverPermissions
      : ([
          [
            "servers.create",
            t("roleDialog.createServersOnSparksApprovedByAnAdministrator"),
          ],
        ] as const);
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
              disabled={busy || deleting || !name.trim()}
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
              setPermissions(next === "server" ? ["server.view"] : []);
            }}
          >
            <option value="server">{t("common.server")}</option>
            <option value="platform">{t("roleDialog.platform")}</option>
          </Select>
        </FormField>
        <h3>{t("roleDialog.permissions")}</h3>
        {choices.map(([permission, label]) => (
          <CheckboxField
            key={permission}
            checked={permissions.includes(permission)}
            disabled={
              busy ||
              deleting ||
              role?.builtin === true ||
              permission === "server.view"
            }
            onChange={(event) =>
              setPermissions(
                event.target.checked
                  ? [...permissions, permission]
                  : permissions.filter((entry) => entry !== permission),
              )
            }
          >
            {label}
          </CheckboxField>
        ))}
        <p className="field-help">
          {t(
            "roleDialog.serverPermissionsNeverGrantAccessToUnassignedServersSparkUser",
          )}
        </p>
      </div>
    </Drawer>
  );
}
