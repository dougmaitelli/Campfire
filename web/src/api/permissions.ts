import { ServerAction } from "../contracts/domains";
import { isEnumValue } from "../utils/validation";
import { apiFetch, readJSON } from "./client";

export const Permission = {
  AllAccess: "*",
  ServersCreate: "servers.create",
  ServersManageAll: "servers.manage_all",
  SparksManage: "sparks.manage",
  EmbersManage: "embers.manage",
  UsersManage: "users.manage",
  RolesManage: "roles.manage",
  AuditView: "audit.view",
  NotificationsRead: "notifications.read",
  NotificationsWrite: "notifications.write",
  NotificationsManage: "notifications.manage",
  ServerView: "server.view",
  ServerOperate: "server.operate",
  ServerConfigure: "server.configure",
  ServerDelete: "server.delete",
  ServerConsoleRead: "server.console.read",
  ServerConsoleWrite: "server.console.write",
  ServerFilesRead: "server.files.read",
  ServerFilesWrite: "server.files.write",
  ServerFilesDelete: "server.files.delete",
  ServerBackupsRead: "server.backups.read",
  ServerBackupsCreate: "server.backups.create",
  ServerBackupsDownload: "server.backups.download",
  ServerBackupsDelete: "server.backups.delete",
  ServerBackupsRestore: "server.backups.restore",
  ServerReinstall: "server.reinstall",
  ServerSchedulesRead: "server.schedules.read",
  ServerSchedulesManage: "server.schedules.manage",
  ServerSchedulesRun: "server.schedules.run",
} as const;

export type PermissionID = (typeof Permission)[keyof typeof Permission];

export function isPermissionID(value: unknown): value is PermissionID {
  return isEnumValue(Permission, value);
}

export function hasPermission(
  values: readonly PermissionID[] | undefined,
  permission: PermissionID | undefined,
): boolean {
  return permission !== undefined && values?.includes(permission) === true;
}

export interface PermissionDefinition {
  id: PermissionID;
  group: string;
  label: string;
  requires: PermissionID[];
}

export function serverActionPermission(
  action: ServerAction,
): PermissionID | undefined {
  switch (action) {
    case ServerAction.Install:
    case ServerAction.Start:
    case ServerAction.Stop:
    case ServerAction.Restart:
    case ServerAction.Kill:
      return Permission.ServerOperate;
    case ServerAction.Update:
    case ServerAction.Reinstall:
      return Permission.ServerReinstall;
    case ServerAction.Backup:
      return Permission.ServerBackupsCreate;
    case ServerAction.Restore:
      return Permission.ServerBackupsRestore;
    case ServerAction.Command:
      return Permission.ServerConsoleWrite;
    default:
      return undefined;
  }
}

export function isPermissionCatalog(
  value: unknown,
): value is PermissionDefinition[] {
  return (
    Array.isArray(value) &&
    value.every((entry: unknown) => {
      if (typeof entry !== "object" || entry === null) return false;
      const item = entry as Record<string, unknown>;
      return (
        isPermissionID(item.id) &&
        typeof item.group === "string" &&
        typeof item.label === "string" &&
        Array.isArray(item.requires) &&
        item.requires.every(isPermissionID)
      );
    })
  );
}

export async function permissionCatalog(
  signal: AbortSignal,
): Promise<PermissionDefinition[]> {
  const value = await readJSON(
    await apiFetch("/api/v1/permissions", { signal }),
  );
  if (!isPermissionCatalog(value))
    throw new Error("Unexpected permission catalog response.");
  return value;
}

export function expandPermissions(
  selected: PermissionID[],
  catalog: PermissionDefinition[],
): PermissionID[] {
  const result = new Set(selected);
  for (;;) {
    const before = result.size;
    for (const item of catalog) {
      if (result.has(item.id))
        for (const dependency of item.requires) result.add(dependency);
    }
    if (result.size === before) return [...result].sort();
  }
}

export function togglePermission(
  selected: PermissionID[],
  id: PermissionID,
  checked: boolean,
  catalog: PermissionDefinition[],
): PermissionID[] {
  if (checked) return expandPermissions([...selected, id], catalog);
  let result = selected.filter((item) => item !== id);
  for (;;) {
    const before = result.length;
    result = result.filter((item) => {
      const definition = catalog.find((entry) => entry.id === item);
      return (
        !definition ||
        definition.requires.every((dependency) => result.includes(dependency))
      );
    });
    if (result.length === before) return result;
  }
}
