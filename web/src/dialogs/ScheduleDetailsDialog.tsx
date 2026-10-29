import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { Schedule } from "../api/activity";
import type { Server } from "../api/servers";
import { Button } from "../components/forms/Button";
import { DetailsList, DetailItem } from "../components/layout/DetailsList";
import { Drawer } from "../components/layout/Drawer";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { ScheduleNextRun } from "../components/schedules/ScheduleNextRun";
import { ScheduleExecutionHistory } from "../components/schedules/ScheduleExecutionHistory";
import { operationLabel } from "../components/servers/operationMessage";
import { StatusPill } from "../components/status/StatusPill";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ScheduleDialog } from "./ScheduleDialog";
import { Permission, serverActionPermission } from "../api/permissions";

export function ScheduleDetailsDialog({
  schedule,
  servers,
  onClose,
  onSaved,
}: {
  schedule: Schedule;
  servers: Server[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const [editing, setEditing] = useState(false);
  const server = servers.find((item) => item.id === schedule.server_id);
  const managed = servers.filter((item) =>
    item.permissions.includes(Permission.ServerSchedulesManage),
  );
  const canEdit = managed.some(
    (item) =>
      item.id === schedule.server_id &&
      item.permissions.includes(serverActionPermission(schedule.action)),
  );
  if (editing && canEdit)
    return (
      <ScheduleDialog
        schedule={schedule}
        servers={managed}
        onClose={onClose}
        onSaved={onSaved}
      />
    );
  return (
    <Drawer
      title={schedule.name}
      description={t("scheduleDetailsDialog.description")}
      busy={false}
      onClose={onClose}
      footerLeft={<Button onClick={onClose}>{t("common.close")}</Button>}
      footerRight={
        canEdit ? (
          <Button variant="primary" onClick={() => setEditing(true)}>
            {t("scheduleDialog.editSchedule")}
          </Button>
        ) : null
      }
    >
      <div className="drawer-content">
        <DetailsList>
          <DetailItem label={t("common.name")}>{schedule.name}</DetailItem>
          <DetailItem label={t("common.server")}>
            <ResourceLink href={links.server(schedule.server_id)}>
              {server?.name ?? schedule.server_id}
            </ResourceLink>
          </DetailItem>
          <DetailItem label={t("scheduleDialog.action")}>
            {operationLabel(schedule.action)}
          </DetailItem>
          <DetailItem label={t("common.status")}>
            <StatusPill tone={schedule.enabled ? "positive" : "warning"}>
              {t(schedule.enabled ? "common.enabled" : "common.disabled")}
            </StatusPill>
          </DetailItem>
          <DetailItem label={t("scheduleDialog.timing")}>
            {t(
              schedule.cron_expression
                ? "scheduleDialog.recurring"
                : "scheduleDialog.runOnce",
            )}
          </DetailItem>
          {schedule.cron_expression && (
            <>
              <DetailItem label={t("scheduleDetailsDialog.cronExpression")}>
                <code>{schedule.cron_expression}</code>
              </DetailItem>
              <DetailItem label={t("scheduleDetailsDialog.timeZone")}>
                {schedule.time_zone}
              </DetailItem>
            </>
          )}
          {!schedule.cron_expression && schedule.next_run_at > 0 && (
            <DetailItem label={t("scheduleDialog.runAt")}>
              <time
                dateTime={new Date(schedule.next_run_at * 1000).toISOString()}
              >
                {new Date(schedule.next_run_at * 1000).toLocaleString()}
              </time>
            </DetailItem>
          )}
          <DetailItem label={t("schedulesPage.nextRun")}>
            <ScheduleNextRun
              at={schedule.next_run_at}
              enabled={schedule.enabled}
            />
          </DetailItem>
          <DetailItem label={t("schedulesPage.runsAs")}>
            <ResourceLink href={links.user(schedule.run_as_user_id)}>
              {schedule.username}
            </ResourceLink>
          </DetailItem>
          <DetailItem label={t("scheduleDialog.onlyRunWhenTheServerIsRunning")}>
            {t(
              schedule.only_when_running ? "common.enabled" : "common.disabled",
            )}
          </DetailItem>
        </DetailsList>
        {schedule.action === "command" && (
          <p className="field-help">
            {t("scheduleDetailsDialog.privateCommand")}
          </p>
        )}
        <ScheduleExecutionHistory key={schedule.id} scheduleID={schedule.id} />
      </div>
    </Drawer>
  );
}
