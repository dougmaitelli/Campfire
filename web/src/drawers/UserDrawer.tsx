import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Select } from "../components/forms/Select";
import { useRef, useState } from "react";
import { deleteUser, saveUser, type Role, type User } from "../api/access";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { CheckboxField } from "../components/forms/CheckboxField";

export function UserDrawer({
  user,
  roles,
  onClose,
  onSaved,
  onDeleted,
}: {
  user: User | null;
  roles: Role[];
  onClose: () => void;
  onSaved: (user: User) => void;
  onDeleted: () => void;
}) {
  const { t } = useTranslation();
  const [username, setUsername] = useState(user?.username ?? "");
  const [name, setName] = useState(user?.name ?? "");
  const [email, setEmail] = useState(user?.email ?? "");
  const emailInput = useRef<HTMLInputElement>(null);
  const [password, setPassword] = useState("");
  const [roleID, setRoleID] = useState(user?.role_id ?? 3);
  const [disabled, setDisabled] = useState(user?.disabled ?? false);
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  async function remove() {
    if (
      !user ||
      pending.current ||
      !window.confirm(
        i18n.t(
          "userDrawer.deleteTransferTheirOwnedServersFirstTheirAccessAndCredentials",
          { userUsername: user.username },
        ),
      )
    )
      return;
    pending.current = true;
    setBusy(true);
    setDeleting(true);
    setError(null);
    try {
      await deleteUser(user.id);
      onDeleted();
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("userDrawer.unableToDeleteUser"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
      setDeleting(false);
    }
  }
  async function submit() {
    if (pending.current) return;
    if (emailInput.current && !emailInput.current.reportValidity()) return;
    if ((!user && !password) || (password && [...password].length < 15)) {
      setError(i18n.t("userDrawer.useAPasswordOfAtLeast15Characters"));
      return;
    }
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      onSaved(
        await saveUser(
          { username, name, email, password, role_id: roleID, disabled },
          user?.id,
        ),
      );
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("userDrawer.unableToSaveUser"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={user ? t("userDrawer.editUser") : t("userDrawer.createUser")}
      description={t("userDrawer.manageThisAccountSRoleAndSignInAccess")}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy} onClick={onClose}>
          {t("common.cancel")}
        </Button>
      }
      footerRight={
        <>
          <DrawerDeleteButton
            onDelete={
              user
                ? () => {
                    void remove();
                  }
                : undefined
            }
            disabled={busy}
            deleting={deleting}
          />
          <Button
            variant="primary"
            disabled={busy || !username.trim()}
            onClick={() => {
              void submit();
            }}
            loading={busy && !deleting}
            loadingLabel={t("userDrawer.saving")}
          >
            {t("userDrawer.saveUser")}
          </Button>
        </>
      }
    >
      <div className="drawer-content">
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {user && (
          <p className="field-help">
            <Trans
              i18nKey="userDrawer.userID"
              t={t}
              components={{ code: <code>{user.id}</code> }}
            />
          </p>
        )}
        <FormField label={t("userDrawer.username")}>
          <TextInput
            id="user-name"
            value={username}
            readOnly={user !== null}
            maxLength={64}
            autoComplete="off"
            onChange={(event) => setUsername(event.target.value)}
          />
        </FormField>
        <FormField
          label={t("userDrawer.nameOptional")}
          description={
            user?.oidc_name_managed
              ? t("userDrawer.managedByYourIdentityProvider")
              : t("userDrawer.shownInTheAccountMenuDefaultsToTheUsername")
          }
        >
          <TextInput
            id="user-display-name"
            readOnly={user?.oidc_name_managed}
            value={name}
            maxLength={100}
            autoComplete="name"
            onChange={(event) => setName(event.target.value)}
          />
        </FormField>
        <FormField
          label={t("userDrawer.emailOptional")}
          description={
            user?.oidc_email_managed
              ? t("userDrawer.managedByYourIdentityProvider")
              : undefined
          }
        >
          <TextInput
            ref={emailInput}
            id="user-email"
            readOnly={user?.oidc_email_managed}
            type="email"
            value={email}
            maxLength={254}
            autoComplete="email"
            onChange={(event) => setEmail(event.target.value)}
          />
        </FormField>
        <FormField
          label={
            <>
              {user
                ? t("userDrawer.newPasswordOptional")
                : t("userDrawer.password")}
            </>
          }
          description={t(
            "userDrawer.atLeast15CharactersPasswordRoleAndAccessChangesRevoke",
          )}
        >
          <TextInput
            id="user-password"
            type="password"
            value={password}
            maxLength={1024}
            autoComplete="new-password"
            onChange={(event) => setPassword(event.target.value)}
          />
        </FormField>
        <FormField label={t("common.role")}>
          <Select
            id="user-role"
            value={roleID}
            onValueChange={(selectedValue) => setRoleID(Number(selectedValue))}
          >
            {roles
              .filter((role) => role.scope === "platform")
              .map((role) => (
                <option key={role.id} value={role.id}>
                  {role.name}
                </option>
              ))}
          </Select>
        </FormField>
        <CheckboxField
          checked={disabled}
          disabled={busy}
          onChange={(event) => setDisabled(event.target.checked)}
        >
          {t("common.disabled")}
        </CheckboxField>
        <p className="field-help">
          {t("userDrawer.usersAccessServersThroughOwnershipOrAServerRoleOnly")}
        </p>
      </div>
    </Drawer>
  );
}
