import type { User } from "../api/access";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { StatusPill } from "../components/status/StatusPill";

export function UserDetailsDialog({
  user,
  onClose,
  onEdit,
  onDelete,
  busy,
  error,
}: {
  user: User;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void;
  busy: boolean;
  error: string | null;
}) {
  return (
    <Drawer
      title={user.name || user.username}
      description="User details and sign-in access."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <div className="row-actions">
          <DrawerDeleteButton onDelete={onDelete} deleting={busy} />
          <button
            type="button"
            className="button"
            disabled={busy}
            onClick={onClose}
          >
            Close
          </button>
        </div>
      }
      footerRight={
        <button
          type="button"
          className="button button-primary"
          onClick={onEdit}
          disabled={busy}
        >
          Edit user
        </button>
      }
    >
      <div className="drawer-content">
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        <dl className="review-grid">
          <div>
            <dt>User ID</dt>
            <dd>{user.id}</dd>
          </div>
          <div>
            <dt>Name</dt>
            <dd>{user.name || "Not set"}</dd>
          </div>
          <div>
            <dt>Username</dt>
            <dd>{user.username}</dd>
          </div>
          <div>
            <dt>Platform role</dt>
            <dd>{user.role.name}</dd>
          </div>
          <div>
            <dt>Status</dt>
            <dd>
              <StatusPill status={user.disabled ? "disabled" : "enabled"}>
                {user.disabled ? "Disabled" : "Enabled"}
              </StatusPill>
            </dd>
          </div>
          <div>
            <dt>Created</dt>
            <dd>
              <time dateTime={new Date(user.created_at * 1000).toISOString()}>
                {new Date(user.created_at * 1000).toLocaleString()}
              </time>
            </dd>
          </div>
        </dl>
      </div>
    </Drawer>
  );
}
