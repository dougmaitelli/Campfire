import { useEffect, useRef, useState } from "react";
import {
  listUsers,
  readServerAccess,
  removeMember,
  shareServer,
  transferOwner,
  type ServerAccess,
  type User,
} from "../api/access";
import type { Server } from "../api/servers";
import { useAuthentication } from "../hooks/useAuthentication";
import { Drawer } from "../components/layout/Drawer";

export function ServerAccessDialog({
  server,
  onClose,
  onChanged,
}: {
  server: Server;
  onClose: () => void;
  onChanged: () => void;
}) {
  const admin = useAuthentication().session?.admin === true;
  const [access, setAccess] = useState<ServerAccess | null>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [username, setUsername] = useState("");
  const [roleID, setRoleID] = useState(5);
  const [ownerID, setOwnerID] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([
      readServerAccess(server.id, controller.signal),
      admin ? listUsers(controller.signal) : Promise.resolve([]),
    ])
      .then(([value, accounts]) => {
        if (!controller.signal.aborted) {
          setAccess(value);
          setUsers(accounts);
          setOwnerID(value.owner?.id ?? 0);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : "Unable to load access.",
          );
      });
    return () => controller.abort();
  }, [admin, server.id]);
  async function change(action: () => Promise<void>) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await action();
      setAccess(await readServerAccess(server.id, AbortSignal.timeout(15000)));
      onChanged();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to change access.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={`Access to ${server.name}`}
      description="The owner controls this server. Collaborators receive a server role."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <button className="button" disabled={busy} onClick={onClose}>
          Close
        </button>
      }
      footerRight={null}
    >
      <div className="drawer-content">
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        {!access ? (
          <p role="status">Loading access…</p>
        ) : (
          <>
            <h3>Owner</h3>
            <p>{access.owner?.username ?? "Administrator managed"}</p>
            {admin && (
              <>
                <label htmlFor="server-owner">Transfer ownership</label>
                <select
                  id="server-owner"
                  value={ownerID}
                  onChange={(event) => setOwnerID(Number(event.target.value))}
                >
                  <option value={0}>Select an account</option>
                  {users
                    .filter((user) => !user.disabled)
                    .map((user) => (
                      <option key={user.id} value={user.id}>
                        {user.username}
                      </option>
                    ))}
                </select>
                <button
                  className="button"
                  disabled={busy || !ownerID || ownerID === access.owner?.id}
                  onClick={() => {
                    if (
                      window.confirm(
                        "Transfer ownership? The previous owner loses owner access.",
                      )
                    )
                      void change(() => transferOwner(server.id, ownerID));
                  }}
                >
                  Transfer
                </button>
              </>
            )}
            <h3>Collaborators</h3>
            {access.members.length === 0 && (
              <p className="field-help">No collaborators assigned.</p>
            )}
            {access.members.map((member) => (
              <div className="access-member" key={member.user_id}>
                <span>
                  {member.username} · {member.role}
                </span>
                <button
                  className="button"
                  disabled={busy}
                  onClick={() => {
                    void change(() => removeMember(server.id, member.user_id));
                  }}
                >
                  Remove
                </button>
              </div>
            ))}
            <label htmlFor="member-name">Existing username</label>
            <input
              id="member-name"
              value={username}
              maxLength={64}
              autoComplete="off"
              onChange={(event) => setUsername(event.target.value)}
            />
            <label htmlFor="member-role">Server role</label>
            <select
              id="member-role"
              value={roleID}
              onChange={(event) => setRoleID(Number(event.target.value))}
            >
              {access.roles.map((role) => (
                <option key={role.id} value={role.id}>
                  {role.name}
                </option>
              ))}
            </select>
            <p className="field-help">
              Operators can use lifecycle controls, console, files, and backups.
              They cannot change configuration, delete the server, or share
              access. Administrators always have access.
            </p>
            <button
              className="button button-primary"
              disabled={busy || !username.trim()}
              onClick={() => {
                void change(async () => {
                  await shareServer(server.id, username, roleID);
                  setUsername("");
                });
              }}
            >
              Assign user
            </button>
          </>
        )}
      </div>
    </Drawer>
  );
}
