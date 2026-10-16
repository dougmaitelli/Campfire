import { useRef, useState } from "react";
import { saveRole, type Role } from "../api/access";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";

const serverPermissions = [
  ["server.view", "View the server and runtime status"],
  ["server.operate", "Lifecycle controls, console, files, and backups"],
  ["server.configure", "Edit server configuration and resource reservations"],
  ["server.delete", "Delete the server and game data"],
  ["server.share", "Share access with operators"],
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
  const [name, setName] = useState(role?.name ?? "");
  const [scope, setScope] = useState<Role["scope"]>(role?.scope ?? "server");
  const [permissions, setPermissions] = useState(
    role?.permissions ?? ["server.view"],
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  async function submit() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      onSaved(await saveRole({ name, scope, permissions }, role?.id));
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to save role.");
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
            "Create servers on Sparks approved by an administrator",
          ],
        ] as const);
  return (
    <Drawer
      title={role ? "Edit role" : "Create role"}
      description="Platform roles grant panel capabilities; server roles apply only to assigned servers."
      busy={busy || deleting}
      onClose={onClose}
      footerLeft={
        <div className="row-actions">
          <DrawerDeleteButton
            onDelete={onDelete}
            disabled={busy}
            deleting={deleting}
          />
          <button
            className="button"
            disabled={busy || deleting}
            onClick={onClose}
          >
            Cancel
          </button>
        </div>
      }
      footerRight={
        <button
          className="button button-primary"
          disabled={busy || deleting || !name.trim()}
          onClick={() => {
            void submit();
          }}
        >
          {busy ? "Saving…" : "Save role"}
        </button>
      }
    >
      <div className="drawer-content">
        {deleteError && (
          <p className="error-message" role="alert">
            {deleteError}
          </p>
        )}
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        <label htmlFor="role-name">Name</label>
        <input
          id="role-name"
          value={name}
          maxLength={64}
          onChange={(event) => setName(event.target.value)}
        />
        <label htmlFor="role-scope">Scope</label>
        <select
          id="role-scope"
          value={scope}
          disabled={role !== null}
          onChange={(event) => {
            const next =
              event.target.value === "platform" ? "platform" : "server";
            setScope(next);
            setPermissions(next === "server" ? ["server.view"] : []);
          }}
        >
          <option value="server">Server</option>
          <option value="platform">Platform</option>
        </select>
        <h3>Permissions</h3>
        {choices.map(([permission, label]) => (
          <label className="checkbox-field" key={permission}>
            <input
              type="checkbox"
              checked={permissions.includes(permission)}
              disabled={permission === "server.view"}
              onChange={(event) =>
                setPermissions(
                  event.target.checked
                    ? [...permissions, permission]
                    : permissions.filter((entry) => entry !== permission),
                )
              }
            />
            {label}
          </label>
        ))}
        <p className="field-help">
          Server permissions never grant access to unassigned servers. Spark,
          user, and role administration remain exclusive to the built-in Admin
          role.
        </p>
      </div>
    </Drawer>
  );
}
