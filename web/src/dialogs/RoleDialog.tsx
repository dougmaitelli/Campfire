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
    if (pending.current || role?.builtin) return;
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
      title={role?.builtin ? "View role" : role ? "Edit role" : "Create role"}
      description="Roles grant panel capabilities; server roles apply only to assigned servers."
      busy={busy || deleting}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy || deleting} onClick={onClose}>
          Cancel
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
              loadingLabel={"Saving…"}
            >
              {"Save role"}
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
        <FormField label="Name">
          <TextInput
            id="role-name"
            readOnly={role?.builtin}
            value={name}
            maxLength={64}
            onChange={(event) => setName(event.target.value)}
          />
        </FormField>
        <FormField label="Scope">
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
            <option value="server">Server</option>
            <option value="platform">Platform</option>
          </Select>
        </FormField>
        <h3>Permissions</h3>
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
          Server permissions never grant access to unassigned servers. Spark,
          user, and role administration remain exclusive to the built-in Admin
          role.
        </p>
      </div>
    </Drawer>
  );
}
