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

export function UserDialog({
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
        `Delete "${user.username}"? Transfer their owned servers first. Their access and credentials will be revoked. Their profile will be retained for audit history.`,
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
        cause instanceof Error ? cause.message : "Unable to delete user.",
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
      setError("Use a password of at least 15 characters.");
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
      setError(cause instanceof Error ? cause.message : "Unable to save user.");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={user ? "Edit user" : "Create user"}
      description="Manage this account's role and sign-in access."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy} onClick={onClose}>
          Cancel
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
            loadingLabel={"Saving…"}
          >
            {"Save user"}
          </Button>
        </>
      }
    >
      <div className="drawer-content">
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        {user && (
          <p className="field-help">
            User ID: <code>{user.id}</code>
          </p>
        )}
        <FormField label="Username">
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
          label="Name (optional)"
          description="Shown in the account menu. Defaults to the username."
        >
          <TextInput
            id="user-display-name"
            value={name}
            maxLength={100}
            autoComplete="name"
            onChange={(event) => setName(event.target.value)}
          />
        </FormField>
        <FormField label="Email (optional)">
          <TextInput
            ref={emailInput}
            id="user-email"
            type="email"
            value={email}
            maxLength={254}
            autoComplete="email"
            onChange={(event) => setEmail(event.target.value)}
          />
        </FormField>
        <FormField
          label={<>{user ? "New password (optional)" : "Password"}</>}
          description="At least 15 characters. Password, role, and access changes revoke this user's existing sessions."
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
        <FormField label="Role">
          <Select
            id="user-role"
            value={roleID}
            onChange={(event) => setRoleID(Number(event.target.value))}
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
          Disabled
        </CheckboxField>
        <p className="field-help">
          Users access servers through ownership or a server role. Only
          administrators manage Sparks.
        </p>
      </div>
    </Drawer>
  );
}
