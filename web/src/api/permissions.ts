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

export interface PermissionDefinition {
  id: string;
  group: string;
  label: string;
  requires: string[];
}

export function serverActionPermission(action: string): string {
  switch (action) {
    case "install":
    case "start":
    case "stop":
    case "restart":
    case "kill":
      return Permission.ServerOperate;
    case "update":
    case "reinstall":
      return Permission.ServerReinstall;
    case "backup":
      return Permission.ServerBackupsCreate;
    case "restore":
      return Permission.ServerBackupsRestore;
    case "command":
      return Permission.ServerConsoleWrite;
    default:
      return "";
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
        typeof item.id === "string" &&
        typeof item.group === "string" &&
        typeof item.label === "string" &&
        Array.isArray(item.requires) &&
        item.requires.every((id: unknown) => typeof id === "string")
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
  selected: string[],
  catalog: PermissionDefinition[],
): string[] {
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
  selected: string[],
  id: string,
  checked: boolean,
  catalog: PermissionDefinition[],
): string[] {
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
