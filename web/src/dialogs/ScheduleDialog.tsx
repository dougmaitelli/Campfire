import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Select } from "../components/forms/Select";
import { useRef, useState } from "react";
import type { Server } from "../api/servers";
import { deleteSchedule, saveSchedule, type Schedule } from "../api/activity";
import { Drawer } from "../components/layout/Drawer";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { CronField } from "../components/schedules/CronField";
import { CheckboxField } from "../components/forms/CheckboxField";
import { serverActionPermission } from "../api/permissions";

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
  const { t } = useTranslation();
  function allowedActions(id: string) {
    const selected = servers.find((item) => item.id === id);
    return ["backup", "restart", "start", "stop", "kill", "command"].filter(
      (item) => selected?.permissions.includes(serverActionPermission(item)),
    );
  }
  const [serverID, setServerID] = useState(
    schedule?.server_id ?? servers[0]?.id ?? "",
  );
  const [name, setName] = useState(schedule?.name ?? "");
  const [action, setAction] = useState(
    schedule?.action ??
      allowedActions(schedule?.server_id ?? servers[0]?.id ?? "")[0] ??
      "backup",
  );
  const [command, setCommand] = useState("");
  const [expression, setExpression] = useState(
    schedule?.cron_expression ?? "0 3 * * *",
  );
  const [timeZone, setTimeZone] = useState(
    schedule?.cron_expression
      ? schedule.time_zone
      : Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
  );
  const [once, setOnce] = useState(schedule?.cron_expression === "");
  const [runAt, setRunAt] = useState(() => {
    const date = new Date(
      (schedule?.next_run_at ?? Math.floor(Date.now() / 1000) + 3600) * 1000,
    );
    return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
      .toISOString()
      .slice(0, 16);
  });
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
        i18n.t("scheduleDialog.deleteScheduleHistoryWillBeRetained", {
          scheduleName: schedule.name,
        }),
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
        cause instanceof Error
          ? cause.message
          : i18n.t("scheduleDialog.unableToDeleteSchedule"),
      );
    } finally {
      pending.current = false;
      setDeleting(false);
      setBusy(false);
    }
  }
  async function submit() {
    if (pending.current) return;
    if (!allowedActions(serverID).includes(action)) {
      setError(i18n.t("scheduleDialog.actionNotAllowed"));
      return;
    }
    if (once && enabled && new Date(runAt).getTime() <= Date.now()) {
      setError(i18n.t("scheduleDialog.chooseAFutureDateAndTimeForThisOneOff"));
      return;
    }
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
          cron_expression: once ? "" : expression,
          next_run_at: once ? Math.floor(new Date(runAt).getTime() / 1000) : 0,
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
        cause instanceof Error
          ? cause.message
          : i18n.t("scheduleDialog.unableToSaveSchedule"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={
        schedule
          ? t("scheduleDialog.editSchedule")
          : t("scheduleDialog.createSchedule")
      }
      description={t(
        "scheduleDialog.oneOffAndRecurringOperationsRunWithYourCurrentPermissions",
      )}
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
              schedule
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
              busy ||
              !name.trim() ||
              !serverID ||
              (once
                ? !runAt || !Number.isFinite(new Date(runAt).getTime())
                : validated !== cronKey) ||
              (action === "command" &&
                !command.trim() &&
                schedule?.action !== "command")
            }
            onClick={() => {
              void submit();
            }}
            loading={busy && !deleting}
            loadingLabel={t("scheduleDialog.saving")}
          >
            {t("scheduleDialog.saveSchedule")}
          </Button>
        </>
      }
    >
      <div className="drawer-content">
        {error && <InlineMessage role="alert">{error}</InlineMessage>}
        <FormField label={t("common.name")}>
          <TextInput
            id="schedule-name"
            maxLength={100}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </FormField>
        <FormField label={t("common.server")}>
          <Select
            id="schedule-server"
            value={serverID}
            disabled={schedule !== null}
            onValueChange={(selectedValue) => {
              setServerID(selectedValue);
              if (!allowedActions(selectedValue).includes(action))
                setAction(allowedActions(selectedValue)[0] ?? "");
            }}
          >
            {servers.map((server) => (
              <option key={server.id} value={server.id}>
                {server.name}
              </option>
            ))}
          </Select>
        </FormField>
        <FormField label={t("scheduleDialog.action")}>
          <Select
            id="schedule-action"
            value={action}
            onValueChange={(selectedValue) => setAction(selectedValue)}
          >
            {allowedActions(serverID).map((value) => (
              <option key={value} value={value}>
                {value === "command"
                  ? i18n.t("scheduleDialog.consoleCommand")
                  : value.charAt(0).toUpperCase() + value.slice(1)}
              </option>
            ))}
          </Select>
        </FormField>
        {action === "command" && (
          <>
            <FormField
              label={t("scheduleDialog.consoleCommand")}
              description={t(
                "scheduleDialog.commandsAreStoredPrivatelyForExecutionWhenEditingACommand",
              )}
            >
              <TextInput
                id="schedule-command"
                type="password"
                autoComplete="off"
                value={command}
                maxLength={4096}
                onChange={(e) => setCommand(e.target.value)}
              />
            </FormField>
          </>
        )}
        <FormField label={t("scheduleDialog.timing")}>
          <Select
            value={once ? "once" : "recurring"}
            onValueChange={(selectedValue) => setOnce(selectedValue === "once")}
          >
            <option value="recurring">{t("scheduleDialog.recurring")}</option>
            <option value="once">{t("scheduleDialog.runOnce")}</option>
          </Select>
        </FormField>
        {once ? (
          <FormField
            label={t("scheduleDialog.runAt")}
            description={t(
              "scheduleDialog.dateAndTimeInYourBrowserSLocalTimeZone",
            )}
          >
            <TextInput
              type="datetime-local"
              value={runAt}
              required
              onChange={(event) => setRunAt(event.target.value)}
            />
          </FormField>
        ) : (
          <CronField
            expression={expression}
            timeZone={timeZone}
            onChange={setExpression}
            onTimeZoneChange={setTimeZone}
            onValidated={setValidated}
            disabled={busy}
          />
        )}
        <CheckboxField
          checked={onlyWhenRunning}
          disabled={busy}
          onChange={(event) => setOnlyWhenRunning(event.target.checked)}
        >
          {t("scheduleDialog.onlyRunWhenTheServerIsRunning")}
        </CheckboxField>
        <p className="field-help">
          {t("scheduleDialog.skipThisRunIfTheServerIsStoppedOtherwiseAttempt")}
        </p>
        <CheckboxField
          checked={enabled}
          disabled={busy}
          onChange={(e) => setEnabled(e.target.checked)}
        >
          {t("common.enabled")}
        </CheckboxField>
        <p className="field-help">
          {t(
            "scheduleDialog.workStaysPendingWhileItsSparkIsOfflineMissedRecurring",
          )}
        </p>
      </div>
    </Drawer>
  );
}
