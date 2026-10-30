import { describe, expect, it } from "vitest";
import {
  expandPermissions,
  Permission,
  togglePermission,
  type PermissionDefinition,
} from "../../src/api/permissions";

const catalog: PermissionDefinition[] = [
  { id: Permission.ServerView, group: "Server", label: "View", requires: [] },
  {
    id: Permission.ServerConsoleRead,
    group: "Console",
    label: "Logs",
    requires: [Permission.ServerView],
  },
  {
    id: Permission.ServerOperate,
    group: "Server",
    label: "Operate",
    requires: [Permission.ServerConsoleRead],
  },
  {
    id: Permission.ServerConfigure,
    group: "Server",
    label: "Configure",
    requires: [Permission.ServerOperate],
  },
  {
    id: Permission.ServerReinstall,
    group: "Server",
    label: "Reinstall",
    requires: [Permission.ServerOperate],
  },
  {
    id: Permission.ServerFilesRead,
    group: "Files",
    label: "Read",
    requires: [Permission.ServerOperate],
  },
  {
    id: Permission.ServerFilesWrite,
    group: "Files",
    label: "Write",
    requires: [Permission.ServerFilesRead],
  },
  {
    id: Permission.ServerBackupsRead,
    group: "Backups",
    label: "View backups",
    requires: [Permission.ServerOperate],
  },
  {
    id: Permission.ServerSchedulesRead,
    group: "Schedules",
    label: "View schedules",
    requires: [Permission.ServerOperate],
  },
  {
    id: Permission.ServerSchedulesManage,
    group: "Schedules",
    label: "Manage schedules",
    requires: [Permission.ServerSchedulesRead],
  },
  {
    id: Permission.ServerBackupsRestore,
    group: "Backups",
    label: "Restore",
    requires: [Permission.ServerBackupsRead],
  },
];

describe("permission selection", () => {
  it("allows reinstallation without configuration access", () => {
    const selected = expandPermissions([Permission.ServerReinstall], catalog);
    expect(selected).toContain(Permission.ServerOperate);
    expect(selected).not.toContain(Permission.ServerConfigure);
    const configured = togglePermission(
      selected,
      Permission.ServerConfigure,
      true,
      catalog,
    );
    expect(
      togglePermission(configured, Permission.ServerConfigure, false, catalog),
    ).toEqual(selected);
  });

  it("adds restore dependencies without granting file access or console commands", () => {
    const selected = expandPermissions(
      [Permission.ServerBackupsRestore],
      catalog,
    );
    expect(selected).toEqual(
      expect.arrayContaining([
        Permission.ServerBackupsRead,
        Permission.ServerOperate,
        Permission.ServerView,
      ]),
    );
    expect(selected).not.toContain(Permission.ServerFilesWrite);
    expect(selected).not.toContain(Permission.ServerFilesRead);
    expect(selected).not.toContain(Permission.ServerFilesDelete);
    expect(selected).not.toContain(Permission.ServerConsoleWrite);
    expect(expandPermissions(selected, catalog)).toEqual(selected);
  });

  it("removes dependent grants when a prerequisite is removed", () => {
    const selected = expandPermissions(
      [Permission.ServerBackupsRestore],
      catalog,
    );
    expect(
      togglePermission(selected, Permission.ServerOperate, false, catalog),
    ).toEqual([Permission.ServerConsoleRead, Permission.ServerView]);
    expect(
      togglePermission(
        [Permission.ServerView],
        Permission.ServerFilesWrite,
        true,
        catalog,
      ),
    ).toEqual([
      Permission.ServerConsoleRead,
      Permission.ServerFilesRead,
      Permission.ServerFilesWrite,
      Permission.ServerOperate,
      Permission.ServerView,
    ]);
  });

  it("clears files, backups, and schedules when operation is deselected", () => {
    const selected = expandPermissions(
      [
        Permission.ServerFilesWrite,
        Permission.ServerBackupsRestore,
        Permission.ServerSchedulesManage,
      ],
      catalog,
    );
    expect(
      togglePermission(selected, Permission.ServerOperate, false, catalog),
    ).toEqual([Permission.ServerConsoleRead, Permission.ServerView]);
  });

  it("keeps restore selected when unrelated file writing is removed", () => {
    const selected = expandPermissions(
      [Permission.ServerBackupsRestore, Permission.ServerFilesWrite],
      catalog,
    );
    const next = togglePermission(
      selected,
      Permission.ServerFilesWrite,
      false,
      catalog,
    );
    expect(next).toContain(Permission.ServerBackupsRestore);
    expect(next).not.toContain(Permission.ServerFilesWrite);
  });
});
