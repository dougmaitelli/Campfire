import { ScheduleAction, BackupMode } from "../contracts/domains";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { Schedule } from "../api/activity";
import type { Server } from "../api/servers";
import { Button } from "../components/forms/Button";
import { ToggleSwitch } from "../components/forms/ToggleSwitch";
import { Checkbox } from "../components/forms/Checkbox";
import { DetailsList, DetailItem } from "../components/layout/DetailsList";
import { Drawer } from "../components/layout/Drawer";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { ScheduleNextRun } from "../components/schedules/ScheduleNextRun";
import { ScheduleExecutionHistory } from "../components/schedules/ScheduleExecutionHistory";
import { operationLabel } from "../components/servers/operationMessage";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ScheduleDrawer } from "./ScheduleDrawer";
import {
  Permission,
  serverActionPermission,
  hasPermission,
} from "../api/permissions";

export function ScheduleDetailsDrawer({
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
      hasPermission(item.permissions, serverActionPermission(schedule.action)),
  );
  if (editing && canEdit)
    return (
      <ScheduleDrawer
        schedule={schedule}
        servers={managed}
        onClose={onClose}
        onSaved={onSaved}
      />
    );
  return (
    <Drawer
      title={schedule.name}
      description={t("scheduleDetailsDrawer.description")}
      busy={false}
      onClose={onClose}
      footerLeft={<Button onClick={onClose}>{t("common.close")}</Button>}
      footerRight={
        canEdit ? (
          <Button variant="primary" onClick={() => setEditing(true)}>
            {t("scheduleDrawer.editSchedule")}
          </Button>
        ) : null
      }
    >
      <div className="panel-content">
        <DetailsList>
          <DetailItem label={t("common.name")}>{schedule.name}</DetailItem>
          <DetailItem label={t("common.server")}>
            <ResourceLink href={links.server(schedule.server_id)}>
              {server?.name ?? schedule.server_id}
            </ResourceLink>
          </DetailItem>
          <DetailItem label={t("common.enabled")}>
            <ToggleSwitch
              aria-label={t("common.enabled")}
              checked={schedule.enabled}
              disabled
            />
          </DetailItem>
          <DetailItem label={t("scheduleDrawer.action")}>
            {operationLabel(schedule.action)}
          </DetailItem>
          {schedule.action === ScheduleAction.Backup && (
            <>
              <DetailItem label={t("backupModes.label")}>
                {t(
                  schedule.backup_mode === BackupMode.Savedata
                    ? "backupModes.savedata"
                    : "backupModes.full",
                )}
              </DetailItem>
              <DetailItem label={t("scheduleDrawer.stopServerDuringBackup")}>
                <Checkbox
                  aria-label={t("scheduleDrawer.stopServerDuringBackup")}
                  checked={!!schedule.backup_stop_server}
                  disabled
                />
              </DetailItem>
            </>
          )}
          <DetailItem label={t("scheduleDrawer.recurring")}>
            <Checkbox
              aria-label={t("scheduleDrawer.recurring")}
              checked={!!schedule.cron_expression}
              disabled
            />
          </DetailItem>
          {schedule.cron_expression && (
            <>
              <DetailItem label={t("scheduleDetailsDrawer.cronExpression")}>
                <code>{schedule.cron_expression}</code>
              </DetailItem>
              <DetailItem label={t("scheduleDetailsDrawer.timeZone")}>
                {schedule.time_zone}
              </DetailItem>
            </>
          )}
          {!schedule.cron_expression && schedule.next_run_at > 0 && (
            <DetailItem label={t("scheduleDrawer.runAt")}>
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
          <DetailItem label={t("scheduleDrawer.onlyRunWhenTheServerIsRunning")}>
            <Checkbox
              aria-label={t("scheduleDrawer.onlyRunWhenTheServerIsRunning")}
              checked={schedule.only_when_running}
              disabled
            />
          </DetailItem>
        </DetailsList>
        {schedule.action === ScheduleAction.Command && (
          <p className="field-help">
            {t("scheduleDetailsDrawer.privateCommand")}
          </p>
        )}
        <ScheduleExecutionHistory key={schedule.id} scheduleID={schedule.id} />
      </div>
    </Drawer>
  );
}
