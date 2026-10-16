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
        `Delete "${user.username}"? Transfer their owned servers first. Their server memberships and sessions will be removed.`,
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
      description="Manage this account's platform role and sign-in access."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <button className="button" disabled={busy} onClick={onClose}>
          Cancel
        </button>
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
          <button
            className="button button-primary"
            disabled={busy || !username.trim()}
            onClick={() => {
              void submit();
            }}
          >
            {busy && !deleting ? "Saving…" : "Save user"}
          </button>
        </>
      }
    >
      <div className="drawer-content">
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        {user && (
          <p className="field-help">
            User ID: <code>{user.id}</code>
          </p>
        )}
        <label htmlFor="user-name">Username</label>
        <input
          id="user-name"
          value={username}
          readOnly={user !== null}
          maxLength={64}
          autoComplete="off"
          onChange={(event) => setUsername(event.target.value)}
        />
        <label htmlFor="user-display-name">Name (optional)</label>
        <input
          id="user-display-name"
          value={name}
          maxLength={100}
          autoComplete="name"
          onChange={(event) => setName(event.target.value)}
        />
        <p className="field-help">
          Shown in the account menu. Defaults to the username.
        </p>
        <label htmlFor="user-email">Email (optional)</label>
        <input
          ref={emailInput}
          id="user-email"
          type="email"
          value={email}
          maxLength={254}
          autoComplete="email"
          onChange={(event) => setEmail(event.target.value)}
        />
        <label htmlFor="user-password">
          {user ? "New password (optional)" : "Password"}
        </label>
        <input
          id="user-password"
          type="password"
          value={password}
          maxLength={1024}
          autoComplete="new-password"
          onChange={(event) => setPassword(event.target.value)}
        />
        <p className="field-help">
          At least 15 characters. Password, role, and access changes revoke this
          user's existing sessions.
        </p>
        <label htmlFor="user-role">Platform role</label>
        <select
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
        </select>
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
