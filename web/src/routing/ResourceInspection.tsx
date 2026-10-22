import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import { listUsers, listRoles } from "../api/access";
import { listEmbers } from "../api/embers";
import { listSchedules } from "../api/activity";
import { listServers } from "../api/servers";
import { useCollection } from "../hooks/useCollection";
import { useAuthentication } from "../hooks/useAuthentication";
import { UserDialog } from "../dialogs/UserDialog";
import { RoleDialog } from "../dialogs/RoleDialog";
import { ImportEmberDialog } from "../dialogs/ImportEmberDialog";
import { ScheduleDialog } from "../dialogs/ScheduleDialog";
import { ServerDetailsDialog } from "../dialogs/ServerDetailsDialog";
import { SparkDetailsDialog } from "../dialogs/SparkDetailsDialog";
import { Drawer } from "../components/layout/Drawer";
import { Button } from "../components/forms/Button";
import { ContentState } from "../components/layout/ContentState";
import { InlineMessage } from "../components/status/InlineMessage";

const InspectionContext = createContext<((href: string) => void) | null>(null);

export function useResourceInspection() {
  return useContext(InspectionContext);
}

export function ResourceInspectionProvider({
  children,
}: {
  children: ReactNode;
}) {
  const [targets, setTargets] = useState<string[]>([]);
  const close = () => setTargets((current) => current.slice(0, -1));
  return (
    <InspectionContext.Provider
      value={(href) => setTargets((current) => [...current, href])}
    >
      {children}
      {targets.map((href, index) => (
        <ResourceInspection
          key={`${index}:${href}`}
          href={href}
          onClose={close}
        />
      ))}
    </InspectionContext.Provider>
  );
}

function ResourceInspection({
  href,
  onClose,
}: {
  href: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [kind, encodedID = ""] = href.slice(1).split("/");
  const id = decodeURIComponent(encodedID);
  if (kind === "servers")
    return (
      <ServerDetailsDialog
        id={id}
        title={t("common.server")}
        onClose={onClose}
      />
    );
  if (kind === "sparks")
    return (
      <SparkDetailsDialog id={id} title={t("common.spark")} onClose={onClose} />
    );
  if (kind === "users") return <InspectUser id={id} onClose={onClose} />;
  if (kind === "roles") return <InspectRole id={id} onClose={onClose} />;
  if (kind === "embers") return <InspectEmber id={id} onClose={onClose} />;
  if (kind === "schedules")
    return <InspectSchedule id={id} onClose={onClose} />;
  return null;
}

type InspectionProps = { id: string; onClose: () => void };

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
      title={t("resourceDetailsDialog.details")}
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
          resource={t("resourceDetailsDialog.details")}
          compact
        />
      )}
    </Drawer>
  );
}

function InspectUser({ id, onClose }: InspectionProps) {
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
    <UserDialog
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

function InspectRole({ id, onClose }: InspectionProps) {
  const roles = useCollection(listRoles);
  const role = roles.items.find((item) => String(item.id) === id);
  useCloseMissing(!roles.loading && !roles.error && !role, onClose);
  if (!role) return <InspectionState error={roles.error} onClose={onClose} />;
  return (
    <RoleDialog
      role={role}
      onClose={onClose}
      onSaved={roles.upsert}
      deleting={false}
      deleteError={null}
    />
  );
}

function InspectEmber({ id, onClose }: InspectionProps) {
  const embers = useCollection(listEmbers);
  const ember = embers.items.find((item) => item.id === id);
  useCloseMissing(!embers.loading && !embers.error && !ember, onClose);
  if (!ember) return <InspectionState error={embers.error} onClose={onClose} />;
  return (
    <ImportEmberDialog
      ember={ember}
      onClose={onClose}
      onImported={embers.upsert}
    />
  );
}

function InspectSchedule({ id, onClose }: InspectionProps) {
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
    <ScheduleDialog
      schedule={schedule}
      servers={servers.items}
      onClose={onClose}
      onSaved={() => {
        void schedules.refresh();
      }}
    />
  );
}
