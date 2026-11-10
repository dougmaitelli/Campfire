import { describe, expect, it } from "vitest";
import { Permission, type PermissionID } from "../../src/api/permissions";
import { ServerAction } from "../../src/contracts/domains";
import {
  canAccessAdminResource,
  canManageSchedule,
  canPerformPlatform,
  canPerformServer,
  canPerformServerAction,
  canRetryExecution,
  canRunSchedule,
} from "../../src/permissions";

function server(...permissions: PermissionID[]) {
  return { permissions: [Permission.ServerView, ...permissions] };
}

describe("UI permission policy", () => {
  it("keeps server configuration independent of global integration setup", () => {
    expect(canPerformServer(server(), "configure", { admin: true })).toBe(
      false,
    );
    expect(
      canPerformServer(server(Permission.ServerConfigure), "configure"),
    ).toBe(true);
    expect(
      canPerformPlatform({ admin: true, view_as: {} }, "manageIntegrations"),
    ).toBe(false);
  });
  it("denies missing subjects and never infers server grants from platform administration", () => {
    const admin = { admin: true };
    expect(canPerformPlatform(null, "createServers")).toBe(false);
    expect(canPerformServer(undefined, "operate", admin)).toBe(false);
    expect(canPerformServer(server(), "operate", admin)).toBe(false);
    expect(canPerformServer(server(), "configure", admin)).toBe(false);
    expect(canPerformServer(server(), "manageAccess", admin)).toBe(false);
    expect(
      canPerformServer({ permissions: [Permission.ServerOperate] }, "operate"),
    ).toBe(false);
  });

  it("requires creation and source configuration for Copy, with file reading for file copies", () => {
    const creator = { permissions: [Permission.ServersCreate] };
    const configured = server(Permission.ServerConfigure);
    const files = server(
      Permission.ServerConfigure,
      Permission.ServerFilesRead,
    );
    expect(canPerformServer(configured, "copy", creator)).toBe(true);
    expect(canPerformServer(configured, "copyFiles", creator)).toBe(false);
    expect(canPerformServer(files, "copyFiles", creator)).toBe(true);
    expect(canPerformServer(files, "copyFiles", {})).toBe(false);
    expect(
      canPerformServer(
        server(Permission.ServerFilesRead),
        "copyFiles",
        creator,
      ),
    ).toBe(false);
    expect(canPerformServer(files, "copy", {})).toBe(false);
  });

  it("keeps ownership editing separate from configuration, file and delete authority", () => {
    const owner = { ...server(), can_manage_access: true };
    expect(canPerformServer(owner, "edit")).toBe(true);
    for (const action of ["configure", "writeFiles", "delete"] as const)
      expect(canPerformServer(owner, action)).toBe(false);
    expect(canPerformServer(server(Permission.ServerConfigure), "edit")).toBe(
      true,
    );
  });

  it("requires both platform grants for transfers", () => {
    for (const permission of [
      Permission.SparksManage,
      Permission.ServersManageAll,
    ])
      expect(
        canPerformServer(server(), "transfer", { permissions: [permission] }),
      ).toBe(false);
    const manager = {
      permissions: [Permission.SparksManage, Permission.ServersManageAll],
    };
    expect(canPerformServer(server(), "transfer", manager)).toBe(true);
  });

  it("requires administration for admin pages, owner selection and user preview", () => {
    const manager = { permissions: [Permission.SparksManage] };
    expect(canPerformPlatform(manager, "manageSparks")).toBe(true);
    expect(canPerformPlatform(manager, "administration")).toBe(false);
    expect(canPerformPlatform(manager, "assignServerOwner")).toBe(false);
    expect(canPerformPlatform(manager, "previewUsers")).toBe(false);
    const admin = { ...manager, admin: true };
    expect(canPerformPlatform(admin, "assignServerOwner")).toBe(true);
    expect(canPerformPlatform(admin, "previewUsers")).toBe(true);
    expect(canPerformPlatform({ ...admin, view_as: {} }, "previewUsers")).toBe(
      false,
    );
    expect(canAccessAdminResource(manager, "manageSparks")).toBe(false);
    expect(canAccessAdminResource(admin, "manageSparks")).toBe(true);
  });

  it("keeps runtime operation grants separate from configuration and file access", () => {
    const operator = server(Permission.ServerOperate);
    expect(canPerformServer(operator, "syncConfiguration")).toBe(true);
    expect(
      canPerformServer(server(Permission.ServerConfigure), "syncConfiguration"),
    ).toBe(false);
    for (const action of [
      ServerAction.Start,
      ServerAction.Stop,
      ServerAction.Restart,
    ])
      expect(canPerformServerAction(operator, action)).toBe(true);
    for (const action of [
      ServerAction.Command,
      ServerAction.Backup,
      ServerAction.Restore,
      ServerAction.Reinstall,
      ServerAction.Status,
    ])
      expect(canPerformServerAction(operator, action)).toBe(false);
    expect(
      canPerformServerAction(
        server(Permission.ServerBackupsRestore),
        ServerAction.Restore,
      ),
    ).toBe(true);
    expect(
      canPerformServerAction(
        server(Permission.ServerBackupsCreate),
        ServerAction.Backup,
      ),
    ).toBe(true);
    expect(
      canPerformServerAction(
        server(Permission.ServerReinstall),
        ServerAction.Reinstall,
      ),
    ).toBe(true);
  });

  it("requires schedule authority and authority for the actual scheduled action", () => {
    const creator = server(
      Permission.ServerSchedulesManage,
      Permission.ServerBackupsCreate,
    );
    const runner = server(
      Permission.ServerSchedulesRun,
      Permission.ServerBackupsCreate,
    );
    expect(canManageSchedule(creator, ServerAction.Backup)).toBe(true);
    expect(canManageSchedule(creator, ServerAction.Command)).toBe(false);
    expect(canRunSchedule(creator, ServerAction.Backup)).toBe(false);
    expect(canRunSchedule(runner, ServerAction.Backup)).toBe(true);
    expect(canManageSchedule(runner, ServerAction.Backup)).toBe(false);
    expect(canRunSchedule(runner, ServerAction.Command)).toBe(false);
    expect(canRetryExecution(creator, ServerAction.Backup, false)).toBe(true);
    expect(canRetryExecution(creator, ServerAction.Backup, true)).toBe(false);
    expect(canRetryExecution(runner, ServerAction.Backup, true)).toBe(true);
  });
});
