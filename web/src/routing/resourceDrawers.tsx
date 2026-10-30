import { useTranslation } from "react-i18next";
import { listUsers, listRoles } from "../api/access";
import { listEmbers } from "../api/embers";
import { listSchedules } from "../api/activity";
import { listServers } from "../api/servers";
import { useCollection } from "../hooks/useCollection";
import { useAuthentication } from "../hooks/useAuthentication";
import { UserDrawer } from "../drawers/UserDrawer";
import { RoleDrawer } from "../drawers/RoleDrawer";
import { ImportEmberDrawer } from "../drawers/ImportEmberDrawer";
import { ScheduleDetailsDrawer } from "../drawers/ScheduleDetailsDrawer";
import { ServerDetailsDrawer } from "../drawers/ServerDetailsDrawer";
import { SparkDetailsDrawer } from "../drawers/SparkDetailsDrawer";
import { Drawer } from "../components/layout/Drawer";
import { Button } from "../components/forms/Button";
import { ContentState } from "../components/layout/ContentState";
import { InlineMessage } from "../components/status/InlineMessage";

import type { InspectionProps } from "./routeRegistry";
import { useEffect } from "react";

export function InspectServer({ id, onClose }: InspectionProps) {
  const { t } = useTranslation();
  return (
    <ServerDetailsDrawer id={id} title={t("common.server")} onClose={onClose} />
  );
}

export function InspectSpark({ id, onClose }: InspectionProps) {
  const { t } = useTranslation();
  return (
    <SparkDetailsDrawer id={id} title={t("common.spark")} onClose={onClose} />
  );
}

function useCloseMissing(missing: boolean, onClose: () => void) {
  useEffect(() => {
    if (missing) onClose();
  }, [missing, onClose]);
}

function InspectionState({
  error,
  onClose,
}: {
  error: string | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  return (
    <Drawer
      title={t("resourceDetailsDrawer.details")}
      busy={false}
      onClose={onClose}
      footerLeft={<Button onClick={onClose}>{t("common.close")}</Button>}
      footerRight={null}
    >
      {error ? (
        <InlineMessage role="alert">{error}</InlineMessage>
      ) : (
        <ContentState
          loading
          resource={t("resourceDetailsDrawer.details")}
          compact
        />
      )}
    </Drawer>
  );
}

export function InspectUser({ id, onClose }: InspectionProps) {
  const users = useCollection(listUsers);
  const roles = useCollection(listRoles);
  const auth = useAuthentication();
  const user = users.items.find((item) => String(item.id) === id);
  useCloseMissing(!users.loading && !users.error && !user, onClose);
  if (!user || roles.loading || roles.error)
    return (
      <InspectionState error={users.error ?? roles.error} onClose={onClose} />
    );
  return (
    <UserDrawer
      user={user}
      roles={roles.items}
      onClose={onClose}
      onSaved={(saved) => {
        users.upsert(saved);
        if (saved.id === auth.session?.user_id) void auth.reload();
      }}
      onDeleted={() => {
        if (user.id === auth.session?.user_id) void auth.reload();
      }}
    />
  );
}

export function InspectRole({ id, onClose }: InspectionProps) {
  const roles = useCollection(listRoles);
  const role = roles.items.find((item) => String(item.id) === id);
  useCloseMissing(!roles.loading && !roles.error && !role, onClose);
  if (!role) return <InspectionState error={roles.error} onClose={onClose} />;
  return (
    <RoleDrawer
      role={role}
      onClose={onClose}
      onSaved={roles.upsert}
      deleting={false}
      deleteError={null}
    />
  );
}

export function InspectEmber({ id, onClose }: InspectionProps) {
  const embers = useCollection(listEmbers);
  const ember = embers.items.find((item) => item.id === id);
  useCloseMissing(!embers.loading && !embers.error && !ember, onClose);
  if (!ember) return <InspectionState error={embers.error} onClose={onClose} />;
  return (
    <ImportEmberDrawer
      ember={ember}
      onClose={onClose}
      onImported={embers.upsert}
    />
  );
}

export function InspectSchedule({ id, onClose }: InspectionProps) {
  const schedules = useCollection(listSchedules);
  const servers = useCollection(listServers);
  const schedule = schedules.items.find((item) => item.id === id);
  useCloseMissing(!schedules.loading && !schedules.error && !schedule, onClose);
  if (!schedule || servers.loading || servers.error)
    return (
      <InspectionState
        error={schedules.error ?? servers.error}
        onClose={onClose}
      />
    );
  return (
    <ScheduleDetailsDrawer
      key={schedule.id}
      schedule={schedule}
      servers={servers.items}
      onClose={onClose}
      onSaved={() => {
        void schedules.refresh();
      }}
    />
  );
}
