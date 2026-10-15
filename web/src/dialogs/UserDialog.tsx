import { useRef, useState } from "react";
import { saveUser, type Role, type User } from "../api/access";
import { Drawer } from "../components/layout/Drawer";

export function UserDialog({
  user,
  roles,
  onClose,
  onSaved,
}: {
  user: User | null;
  roles: Role[];
  onClose: () => void;
  onSaved: (user: User) => void;
}) {
  const [username, setUsername] = useState(user?.username ?? "");
  const [password, setPassword] = useState("");
  const [roleID, setRoleID] = useState(user?.role_id ?? 3);
  const [disabled, setDisabled] = useState(user?.disabled ?? false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  async function submit() {
    if (pending.current) return;
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
          { username, password, role_id: roleID, disabled },
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
        <button
          className="button button-primary"
          disabled={busy || !username.trim()}
          onClick={() => {
            void submit();
          }}
        >
          {busy ? "Saving…" : "Save user"}
        </button>
      }
    >
      <div className="drawer-content">
        {error && (
          <p className="error-message" role="alert">
            {error}
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
          At least 15 characters. Account changes revoke this user's existing
          sessions.
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
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={disabled}
            onChange={(event) => setDisabled(event.target.checked)}
          />
          Disabled
        </label>
        <p className="field-help">
          Users access servers through ownership or a server role. Only
          administrators manage Sparks.
        </p>
      </div>
    </Drawer>
  );
}
