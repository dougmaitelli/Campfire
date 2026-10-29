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
    id: Permission.ServerOperate,
    group: "Server",
    label: "Operate",
    requires: [Permission.ServerView],
  },
  {
    id: Permission.ServerFilesRead,
    group: "Files",
    label: "Read",
    requires: [Permission.ServerView],
  },
  {
    id: Permission.ServerFilesWrite,
    group: "Files",
    label: "Write",
    requires: [Permission.ServerFilesRead],
  },
  {
    id: Permission.ServerBackupsRestore,
    group: "Backups",
    label: "Restore",
    requires: [Permission.ServerFilesWrite, Permission.ServerOperate],
  },
];

describe("permission selection", () => {
  it("adds restore dependencies without granting delete or console commands", () => {
    const selected = expandPermissions(
      [Permission.ServerBackupsRestore],
      catalog,
    );
    expect(selected).toEqual(
      expect.arrayContaining([
        Permission.ServerFilesWrite,
        Permission.ServerFilesRead,
        Permission.ServerOperate,
        Permission.ServerView,
      ]),
    );
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
      togglePermission(selected, Permission.ServerFilesRead, false, catalog),
    ).toEqual([Permission.ServerOperate, Permission.ServerView]);
    expect(
      togglePermission(
        [Permission.ServerView],
        Permission.ServerFilesWrite,
        true,
        catalog,
      ),
    ).toEqual([
      Permission.ServerFilesRead,
      Permission.ServerFilesWrite,
      Permission.ServerView,
    ]);
  });
});
