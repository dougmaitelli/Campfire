import { useEffect, useRef, useState } from "react";
import {
  listRoles,
  readSparkRoles,
  setSparkRoles,
  type Role,
} from "../api/access";
import type { Spark } from "../api/sparks";
import { Drawer } from "../components/layout/Drawer";
import { CheckboxField } from "../components/forms/CheckboxField";

export function SparkRolesDialog({
  spark,
  onClose,
}: {
  spark: Spark;
  onClose: () => void;
}) {
  const [roles, setRoles] = useState<Role[]>([]);
  const [selected, setSelected] = useState<number[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([
      listRoles(controller.signal),
      readSparkRoles(spark.id, controller.signal),
    ])
      .then(([catalog, ids]) => {
        if (!controller.signal.aborted) {
          setRoles(
            catalog.filter(
              (role) =>
                role.id !== 1 &&
                role.scope === "platform" &&
                role.permissions.includes("servers.create"),
            ),
          );
          setSelected(ids);
          setLoading(false);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : "Unable to load provisioning access.",
          );
      });
    return () => controller.abort();
  }, [spark.id]);
  async function submit() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await setSparkRoles(
        spark.id,
        selected.filter((id) => roles.some((role) => role.id === id)),
      );
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to save provisioning access.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={`Provisioning on ${spark.name}`}
      description="Choose which platform roles can create servers on this Spark."
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
          disabled={busy || loading}
          onClick={() => {
            void submit();
          }}
        >
          {busy ? "Saving…" : "Save access"}
        </button>
      }
    >
      <div className="drawer-content">
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        {loading ? (
          <p role="status">Loading roles…</p>
        ) : (
          roles.map((role) => (
            <CheckboxField
              key={role.id}
              checked={selected.includes(role.id)}
              disabled={busy}
              onChange={(event) =>
                setSelected(
                  event.target.checked
                    ? [...selected, role.id]
                    : selected.filter((id) => id !== role.id),
                )
              }
            >
              {role.name}
            </CheckboxField>
          ))
        )}
        <p className="field-help">
          Only roles with server creation permission appear. Administrators can
          always provision. These grants do not allow Spark management, and
          removing a grant does not remove ownership of existing servers.
        </p>
      </div>
    </Drawer>
  );
}
