import { useRef, useState } from "react";
import type { Server } from "../api/servers";
import { deleteSchedule, saveSchedule, type Schedule } from "../api/activity";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { CronField } from "../components/schedules/CronField";

export function ScheduleDialog({
  schedule,
  servers,
  onClose,
  onSaved,
}: {
  schedule: Schedule | null;
  servers: Server[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [serverID, setServerID] = useState(
    schedule?.server_id ?? servers[0]?.id ?? "",
  );
  const [name, setName] = useState(schedule?.name ?? "");
  const [action, setAction] = useState(schedule?.action ?? "backup");
  const [command, setCommand] = useState("");
  const [expression, setExpression] = useState(
    schedule?.cron_expression ?? "0 3 * * *",
  );
  const [timeZone, setTimeZone] = useState(
    schedule?.cron_expression
      ? schedule.time_zone
      : Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
  );
  const [validated, setValidated] = useState("");
  const cronKey = `${expression.trim().replace(/\s+/g, " ")}\n${timeZone}`;
  const [enabled, setEnabled] = useState(schedule?.enabled ?? true);
  const [onlyWhenRunning, setOnlyWhenRunning] = useState(
    schedule?.only_when_running ?? false,
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  const [deleting, setDeleting] = useState(false);
  async function remove() {
    if (
      !schedule ||
      pending.current ||
      !window.confirm(
        `Delete schedule "${schedule.name}"? History will be retained.`,
      )
    )
      return;
    pending.current = true;
    setDeleting(true);
    setBusy(true);
    setError(null);
    try {
      await deleteSchedule(schedule.id);
      onSaved();
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to delete schedule.",
      );
    } finally {
      pending.current = false;
      setDeleting(false);
      setBusy(false);
    }
  }
  async function submit() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await saveSchedule(
        {
          server_id: serverID,
          name,
          action,
          command,
          cron_expression: expression,
          time_zone: timeZone,
          enabled,
          only_when_running: onlyWhenRunning,
        },
        schedule?.id,
      );
      onSaved();
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to save schedule.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={schedule ? "Edit schedule" : "Create schedule"}
      description="Recurring server operations run with your current permissions."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <div className="row-actions">
          <DrawerDeleteButton
            onDelete={
              schedule
                ? () => {
                    void remove();
                  }
                : undefined
            }
            disabled={busy}
            deleting={deleting}
          />
          <button className="button" disabled={busy} onClick={onClose}>
            Cancel
          </button>
        </div>
      }
      footerRight={
        <button
          className="button button-primary"
          disabled={
            busy ||
            !name.trim() ||
            !serverID ||
            validated !== cronKey ||
            (action === "command" &&
              !command.trim() &&
              schedule?.action !== "command")
          }
          onClick={() => {
            void submit();
          }}
        >
          {busy && !deleting ? "Saving…" : "Save schedule"}
        </button>
      }
    >
      <div className="drawer-content">
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        <label htmlFor="schedule-name">Name</label>
        <input
          id="schedule-name"
          maxLength={100}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <label htmlFor="schedule-server">Server</label>
        <select
          id="schedule-server"
          value={serverID}
          disabled={schedule !== null}
          onChange={(e) => setServerID(e.target.value)}
        >
          {servers.map((server) => (
            <option key={server.id} value={server.id}>
              {server.name}
            </option>
          ))}
        </select>
        <label htmlFor="schedule-action">Action</label>
        <select
          id="schedule-action"
          value={action}
          onChange={(e) => setAction(e.target.value)}
        >
          {["backup", "restart", "start", "stop", "kill", "command"].map(
            (value) => (
              <option key={value} value={value}>
                {value === "command"
                  ? "Console command"
                  : value.charAt(0).toUpperCase() + value.slice(1)}
              </option>
            ),
          )}
        </select>
        {action === "command" && (
          <>
            <label htmlFor="schedule-command">Console command</label>
            <input
              id="schedule-command"
              type="password"
              autoComplete="off"
              value={command}
              maxLength={4096}
              onChange={(e) => setCommand(e.target.value)}
            />
            <p className="field-help">
              Commands are stored privately for execution. When editing a
              command schedule, leave this blank to keep its stored command.
            </p>
          </>
        )}
        <CronField
          expression={expression}
          timeZone={timeZone}
          onChange={setExpression}
          onTimeZoneChange={setTimeZone}
          onValidated={setValidated}
          disabled={busy}
        />
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={onlyWhenRunning}
            disabled={busy}
            onChange={(event) => setOnlyWhenRunning(event.target.checked)}
          />
          Only run when the server is running
        </label>
        <p className="field-help">
          Skip this run if the server is stopped. Otherwise, attempt the
          scheduled action regardless of server state; each action still has its
          normal requirements.
        </p>
        <label className="checkbox-field">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          Enabled
        </label>
        <p className="field-help">
          Missed runs produce one execution after downtime. Backups require a
          stopped server. Editing makes you the execution owner.
        </p>
      </div>
    </Drawer>
  );
}
