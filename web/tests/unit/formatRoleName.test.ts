import { expect, test } from "vitest";
import i18n from "../../src/i18n";
import { RoleScope } from "../../src/contracts/domains";
import { RoleLimits } from "../../src/config/policies";
import { serverPermissionPresets } from "../../src/config/permissionPresets";
import {
  Permission,
  expandPermissions,
  type PermissionDefinition,
  type PermissionID,
} from "../../src/api/permissions";
import { formatRoleName } from "../../src/utils/formatRoleName";

const catalog: PermissionDefinition[] = [
  {
    id: Permission.ServerView,
    group: "Server",
    label: "View server",
    requires: [],
  },
  ...(
    [
      [Permission.ServerConsoleRead, Permission.ServerView],
      [Permission.ServerOperate, Permission.ServerConsoleRead],
      [Permission.ServerConsoleWrite, Permission.ServerOperate],
      [Permission.ServerConfigure, Permission.ServerOperate],
      [Permission.ServerReinstall, Permission.ServerOperate],
      [Permission.ServerFilesRead, Permission.ServerOperate],
      [Permission.ServerFilesWrite, Permission.ServerFilesRead],
      [Permission.ServerFilesDelete, Permission.ServerFilesWrite],
      [Permission.ServerBackupsRead, Permission.ServerOperate],
      [Permission.ServerBackupsCreate, Permission.ServerBackupsRead],
      [Permission.ServerBackupsRestore, Permission.ServerBackupsRead],
      [Permission.ServerBackupsDownload, Permission.ServerBackupsRead],
      [Permission.ServerBackupsDelete, Permission.ServerBackupsRead],
      [Permission.ServerSchedulesRead, Permission.ServerOperate],
      [Permission.ServerSchedulesManage, Permission.ServerSchedulesRead],
      [Permission.ServerSchedulesRun, Permission.ServerSchedulesRead],
      [Permission.ServerDelete, Permission.ServerOperate],
    ] as const
  ).map(([id, parent]) => ({
    id,
    group: "Server",
    label: id,
    requires: [parent],
  })),
];

function serverName(grants: PermissionID[]) {
  return formatRoleName({
    scope: RoleScope.Server,
    permissions: expandPermissions(grants, catalog),
    catalog,
    existingRoles: [],
  });
}

test("role suggestions use exact preset names and a viewing-only default", () => {
  expect(serverName([Permission.ServerView])).toBe("Server viewer");
  for (const [label, grants] of serverPermissionPresets(catalog))
    expect(serverName([...grants])).toBe(label);
});

test("custom role names describe leaf grants rather than repeating their prerequisites", () => {
  expect(
    serverName([Permission.ServerFilesWrite, Permission.ServerBackupsRestore]),
  ).toBe("Server · Write files + Restore backups");
});

test("adding permissions beyond a preset does not keep its preset suggestion", () => {
  const administrator = catalog.map((item) => item.id);
  expect(serverName(administrator)).not.toBe("Server administrator");
});

test("platform role suggestions describe their available grants", () => {
  const input = { scope: RoleScope.Platform, catalog, existingRoles: [] };
  expect(formatRoleName({ ...input, permissions: [] })).toBe("Platform member");
  expect(
    formatRoleName({ ...input, permissions: [Permission.ServersCreate] }),
  ).toBe("Server creator");
});

test("role suggestions avoid case-insensitive collisions within their own scope", () => {
  const permissions = expandPermissions(
    [
      Permission.ServerBackupsCreate,
      Permission.ServerBackupsRestore,
      Permission.ServerBackupsDownload,
    ],
    catalog,
  );
  expect(
    formatRoleName({
      scope: RoleScope.Server,
      permissions,
      catalog,
      existingRoles: [
        { name: "BACKUP MAINTAINER", scope: RoleScope.Server },
        { name: "Backup maintainer 2", scope: RoleScope.Server },
        { name: "Backup maintainer 3", scope: RoleScope.Platform },
      ],
    }),
  ).toBe("Backup maintainer 3");
});

test("long translated suggestions and their numbered alternatives respect the role name limit", async () => {
  i18n.addResourceBundle(
    "test",
    "translation",
    { permissionNames: { server_files_write: "🌲".repeat(100) } },
    true,
    true,
  );
  await i18n.changeLanguage("test");
  const input = {
    scope: RoleScope.Server,
    permissions: expandPermissions([Permission.ServerFilesWrite], catalog),
    catalog,
  };
  const first = formatRoleName({ ...input, existingRoles: [] });
  const second = formatRoleName({
    ...input,
    existingRoles: [{ name: first, scope: RoleScope.Server }],
  });
  expect(Array.from(first)).toHaveLength(RoleLimits.NameLength);
  expect(Array.from(second)).toHaveLength(RoleLimits.NameLength);
  expect(second.endsWith(" 2")).toBe(true);
  expect(second).not.toContain("\uFFFD");
});
