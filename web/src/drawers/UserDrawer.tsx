import { ProfileMFA } from "../components/navigation/ProfileMFA";
import type { MFAState } from "../hooks/useMFAStatus";
import { saveProfile } from "../api/profile";
import { AdminMFAReset } from "../components/navigation/AdminMFAReset";
import { RoleScope } from "../contracts/domains";
import { AccountLimits, BuiltinRole } from "../config/policies";
import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { EmailInput } from "../components/forms/EmailInput";
import {
  PasswordInput,
  passwordPolicy,
} from "../components/forms/PasswordInput";
import { Select } from "../components/forms/Select";
import { lazy, Suspense, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { deleteUser, saveUser, type Role, type User } from "../api/access";
import { Drawer } from "../components/layout/Drawer";
import { InputDialog } from "../components/layout/InputDialog";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { ToggleSwitch } from "../components/forms/ToggleSwitch";
import { useAuthentication } from "../hooks/useAuthentication";
import { useConfirmation } from "../hooks/useConfirmation";

const APITokensDrawer = lazy(() =>
  import("./APITokensDrawer").then((module) => ({
    default: module.APITokensDrawer,
  })),
);
const SessionsDrawer = lazy(() =>
  import("./SessionsDrawer").then((module) => ({
    default: module.SessionsDrawer,
  })),
);

export function UserDrawer({
  user,
  roles,
  onClose,
  onSaved,
  onDeleted,
  profile,
}: {
  profile?: {
    passwordAllowed: boolean;
    mfa: MFAState;
  };
  user: User | null;
  roles: Role[];
  onClose: () => void;
  onSaved: (user: User) => void;
  onDeleted: () => void;
}) {
  const { t } = useTranslation();
  const confirmation = useConfirmation();
  const auth = useAuthentication();
  const { session } = auth;
  const canPreProvision = !user && !!session?.oidc_auto_link_email;
  const showPassword = profile
    ? profile.passwordAllowed
    : (!user && !canPreProvision) || session?.local_login !== false;
  const [username, setUsername] = useState(user?.username ?? "");
  const [originalUser] = useState(user);
  const [name, setName] = useState(user?.name ?? "");
  const [email, setEmail] = useState(user?.email ?? "");
  const providerName = !!user?.oidc_name_managed;
  const providerEmail = !!user?.oidc_email_managed;
  const emailInput = useRef<HTMLInputElement>(null);
  const [password, setPassword] = useState("");
  const [repeatPassword, setRepeatPassword] = useState("");
  const [roleID, setRoleID] = useState(user?.role_id ?? BuiltinRole.User);
  const [disabled, setDisabled] = useState(user?.disabled ?? false);
  const [saving, setBusy] = useState(false);
  const [securityBusy, setSecurityBusy] = useState(false);
  const [accountDrawer, setAccountDrawer] = useState<
    "tokens" | "sessions" | null
  >(null);
  const [currentPassword, setCurrentPassword] = useState("");
  const [factorPromptOpen, setFactorPromptOpen] = useState(false);
  const busy = saving || securityBusy || !!profile?.mfa.loading;
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  async function remove() {
    if (
      !user ||
      pending.current ||
      !(await confirmation.confirm({
        title: t("confirmDialog.deleteUser"),
        description: i18n.t(
          "userDrawer.deleteTransferTheirOwnedServersFirstTheirAccessAndCredentials",
          { userUsername: user.username },
        ),
        confirmLabel: t("common.delete"),
        danger: true,
      }))
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
  async function submit(factorCode?: string) {
    if (pending.current || (profile && !profile.mfa.status)) return;
    if (
      (!user || email !== originalUser?.email) &&
      !emailInput.current?.reportValidity()
    )
      return;
    const passwordToSave = showPassword ? password : "";
    if (profile && passwordToSave && passwordToSave !== repeatPassword) {
      setError(t("profile.passwordMismatch"));
      return;
    }
    if (
      (!user && !canPreProvision && !passwordToSave) ||
      (passwordToSave &&
        !Object.values(passwordPolicy(passwordToSave)).every(Boolean))
    ) {
      setError(t("passwordInput.invalid"));
      return;
    }
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      if (
        profile?.mfa.status?.enabled &&
        passwordToSave &&
        factorCode === undefined
      ) {
        setFactorPromptOpen(true);
        return;
      }
      const saved = profile
        ? await saveProfile(
            providerName || name === originalUser?.name ? undefined : name,
            providerEmail || email === originalUser?.email ? undefined : email,
            passwordToSave,
            currentPassword,
            factorCode ?? "",
          )
        : await saveUser(
            {
              username,
              name,
              email,
              password: passwordToSave,
              role_id: roleID,
              disabled,
            },
            originalUser,
          );
      if (saved) onSaved(saved);
      onClose();
    } catch (cause) {
      if (factorCode !== undefined) throw cause;
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
      actions={
        profile
          ? [
              {
                label: t("apiTokensDrawer.apiTokens"),
                icon: "key",
                disabled: busy,
                onSelect: () => setAccountDrawer("tokens"),
              },
              {
                label: t("sessions.title"),
                icon: "devices",
                disabled: busy,
                onSelect: () => setAccountDrawer("sessions"),
              },
            ]
          : undefined
      }
      title={
        profile
          ? t("profile.title")
          : user
            ? t("userDrawer.editUser")
            : t("userDrawer.createUser")
      }
      description={
        profile
          ? t("profile.description")
          : t("userDrawer.manageThisAccountSRoleAndSignInAccess")
      }
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
              user && !profile
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
            disabled={
              busy || !username.trim() || !!(profile && !profile.mfa.status)
            }
            onClick={() => {
              void submit();
            }}
            loading={saving && !deleting}
            loadingLabel={t("userDrawer.saving")}
          >
            {t(profile ? "profile.save" : "userDrawer.saveUser")}
          </Button>
        </>
      }
    >
      <div className="panel-content">
        {confirmation.dialog}
        {factorPromptOpen && (
          <InputDialog
            title={t("profile.verifyPasswordChange")}
            label={t("mfa.code")}
            description={t("profile.enterFreshCode")}
            submitLabel={t("profile.save")}
            onClose={() => setFactorPromptOpen(false)}
            onSubmit={submit}
          />
        )}
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {user && !profile && (
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
            maxLength={AccountLimits.UsernameMax}
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
            readOnly={providerName}
            value={name}
            maxLength={100}
            autoComplete="name"
            onChange={(event) => setName(event.target.value)}
          />
        </FormField>
        <FormField
          label={t("email.address")}
          description={
            user?.oidc_email_managed
              ? t("userDrawer.managedByYourIdentityProvider")
              : undefined
          }
        >
          <EmailInput
            ref={emailInput}
            id="user-email"
            readOnly={providerEmail}
            required
            value={email}
            maxLength={254}
            onChange={(event) => setEmail(event.target.value)}
          />
        </FormField>
        {showPassword && (
          <FormField
            label={
              <>
                {user
                  ? t("userDrawer.newPasswordOptional")
                  : t(
                      canPreProvision
                        ? "userDrawer.optionalPassword"
                        : "userDrawer.password",
                    )}
              </>
            }
            description={
              password
                ? t(
                    profile
                      ? "profile.passwordEffects"
                      : "userDrawer.atLeast15CharactersPasswordRoleAndAccessChangesRevoke",
                  )
                : canPreProvision
                  ? t("userDrawer.oidcPasswordHint")
                  : undefined
            }
          >
            <PasswordInput
              id="user-password"
              value={password}
              autoComplete="new-password"
              onChange={(event) => {
                setPassword(event.target.value);
                if (!event.target.value) {
                  setRepeatPassword("");
                  setCurrentPassword("");
                  setError(null);
                }
              }}
            />
          </FormField>
        )}
        {profile && showPassword && password && (
          <>
            <FormField label={t("profile.repeatPassword")}>
              <TextInput
                type="password"
                autoComplete="new-password"
                maxLength={AccountLimits.PasswordMax}
                disabled={busy}
                value={repeatPassword}
                onChange={(event) => setRepeatPassword(event.target.value)}
              />
            </FormField>
            <FormField label={t("profile.currentPassword")}>
              <TextInput
                type="password"
                autoComplete="current-password"
                maxLength={1024}
                disabled={busy}
                value={currentPassword}
                onChange={(event) => setCurrentPassword(event.target.value)}
              />
            </FormField>
          </>
        )}
        {!profile && (
          <>
            <FormField label={t("common.platformRole")}>
              <Select
                id="user-role"
                value={roleID}
                onValueChange={(selectedValue) =>
                  setRoleID(Number(selectedValue))
                }
              >
                {roles
                  .filter((role) => role.scope === RoleScope.Platform)
                  .map((role) => (
                    <option key={role.id} value={role.id}>
                      {role.name}
                    </option>
                  ))}
              </Select>
            </FormField>
            <FormField label={t("common.enabled")}>
              <ToggleSwitch
                aria-label={t("common.enabled")}
                checked={!disabled}
                disabled={busy}
                onChange={(event) => setDisabled(!event.target.checked)}
              />
            </FormField>
          </>
        )}
        {profile &&
          accountDrawer &&
          createPortal(
            <Suspense fallback={null}>
              {accountDrawer === "tokens" ? (
                <APITokensDrawer onClose={() => setAccountDrawer(null)} />
              ) : (
                <SessionsDrawer onClose={() => setAccountDrawer(null)} />
              )}
            </Suspense>,
            document.body,
          )}
        {profile && (
          <ProfileMFA
            state={profile.mfa}
            disabled={busy}
            onBusy={setSecurityBusy}
          />
        )}
        {user &&
          session?.admin &&
          !session.view_as &&
          user.id !== session.user_id && (
            <AdminMFAReset
              userID={user.id}
              username={user.username}
              onBusy={setSecurityBusy}
            />
          )}
      </div>
    </Drawer>
  );
}
