import { Permission, type PermissionDefinition } from "../api/permissions";

export function serverPermissionPresets(
  catalog: readonly PermissionDefinition[],
) {
  return [
    ["Log viewer", [Permission.ServerConsoleRead]],
    ["Basic operator", [Permission.ServerOperate]],
    [
      "Backup maintainer",
      [
        Permission.ServerBackupsCreate,
        Permission.ServerBackupsRestore,
        Permission.ServerBackupsDownload,
      ],
    ],
    [
      "Server administrator",
      catalog
        .filter((item) => item.id !== Permission.ServerDelete)
        .map((item) => item.id),
    ],
  ] as const;
}
