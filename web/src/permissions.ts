import { Permission, type PermissionID } from "./api/permissions";
import { ServerAction } from "./contracts/domains";

interface Principal {
  admin?: boolean;
  permissions?: readonly PermissionID[];
  view_as?: unknown;
}

interface ServerSubject {
  permissions: readonly PermissionID[];
  can_manage_access?: boolean;
}

const platformRequirements = {
  createServers: [Permission.ServersCreate],
  manageAllServers: [Permission.ServersManageAll],
  manageSparks: [Permission.SparksManage],
  manageEmbers: [Permission.EmbersManage],
  manageUsers: [Permission.UsersManage],
  manageRoles: [Permission.RolesManage],
  transferServers: [Permission.SparksManage, Permission.ServersManageAll],
} as const;

export type AdminResourceOperation =
  "manageSparks" | "manageEmbers" | "manageUsers" | "manageRoles";

const serverRequirements = {
  view: [Permission.ServerView],
  operate: [Permission.ServerOperate],
  configure: [Permission.ServerConfigure],
  delete: [Permission.ServerDelete],
  reinstall: [Permission.ServerReinstall],
  readConsole: [Permission.ServerConsoleRead],
  writeConsole: [Permission.ServerConsoleWrite],
  readFiles: [Permission.ServerFilesRead],
  writeFiles: [Permission.ServerFilesWrite],
  deleteFiles: [Permission.ServerFilesDelete],
  readBackups: [Permission.ServerBackupsRead],
  createBackups: [Permission.ServerBackupsCreate],
  restoreBackups: [Permission.ServerBackupsRestore],
  downloadBackups: [Permission.ServerBackupsDownload],
  deleteBackups: [Permission.ServerBackupsDelete],
  readSchedules: [Permission.ServerSchedulesRead],
  manageSchedules: [Permission.ServerSchedulesManage],
  runSchedules: [Permission.ServerSchedulesRun],
  copy: [Permission.ServerConfigure],
  copyFiles: [Permission.ServerConfigure, Permission.ServerFilesRead],
} as const;

type ServerOperation =
  | keyof typeof serverRequirements
  | "manageAccess"
  | "edit"
  | "viewJobs"
  | "transfer"
  | "updateEmber";

export function canPerformPlatform(
  principal: Principal | null | undefined,
  operation:
    | keyof typeof platformRequirements
    | "administration"
    | "assignServerOwner"
    | "manageIntegrations"
    | "previewUsers",
): boolean {
  switch (operation) {
    case "administration":
    case "assignServerOwner":
      return principal?.admin === true;
    case "previewUsers":
    case "manageIntegrations":
      return principal?.admin === true && !principal.view_as;
  }
  return (
    principal?.admin === true ||
    platformRequirements[operation].every(
      (permission) => principal?.permissions?.includes(permission) === true,
    )
  );
}

export function canAccessAdminResource(
  principal: Principal | null | undefined,
  operation: AdminResourceOperation,
): boolean {
  return (
    canPerformPlatform(principal, "administration") &&
    platformRequirements[operation].every(
      (permission) => principal?.permissions?.includes(permission) === true,
    )
  );
}

// Server grants are already expanded and restricted by the API. A platform
// administrator flag must never override the grants returned for a server.
export function canPerformServer(
  server: ServerSubject | null | undefined,
  operation: ServerOperation,
  principal?: Principal | null,
): boolean {
  if (!server?.permissions.includes(Permission.ServerView)) return false;
  switch (operation) {
    case "manageAccess":
      return server.can_manage_access === true;
    case "edit":
      return (
        canPerformServer(server, "configure") ||
        canPerformServer(server, "manageAccess")
      );
    case "viewJobs":
      return (
        canPerformServer(server, "configure") ||
        canPerformServer(server, "transfer", principal)
      );
    case "transfer":
      return canPerformPlatform(principal, "transferServers");
    case "updateEmber":
      return canPerformPlatform(principal, "manageAllServers");
    default:
      return (
        serverRequirements[operation].every((permission) =>
          server.permissions.includes(permission),
        ) &&
        (!(operation === "copy" || operation === "copyFiles") ||
          canPerformPlatform(principal, "createServers"))
      );
  }
}

function serverActionPermission(
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

export function canPerformServerAction(
  server: ServerSubject | null | undefined,
  action: ServerAction,
): boolean {
  const permission = serverActionPermission(action);
  return (
    canPerformServer(server, "view") &&
    permission !== undefined &&
    server?.permissions.includes(permission) === true
  );
}

export function canManageSchedule(
  server: ServerSubject | null | undefined,
  action: ServerAction,
): boolean {
  return (
    canPerformServer(server, "manageSchedules") &&
    canPerformServerAction(server, action)
  );
}

export function canRunSchedule(
  server: ServerSubject | null | undefined,
  action: ServerAction,
): boolean {
  return (
    canPerformServer(server, "runSchedules") &&
    canPerformServerAction(server, action)
  );
}

export function canRetryExecution(
  server: ServerSubject | null | undefined,
  action: ServerAction,
  scheduled: boolean,
): boolean {
  return scheduled
    ? canRunSchedule(server, action)
    : canPerformServerAction(server, action);
}
