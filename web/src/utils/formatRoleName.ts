import { RoleScope } from "../contracts/domains";
import {
  Permission,
  expandPermissions,
  type PermissionDefinition,
  type PermissionID,
} from "../api/permissions";
import { serverPermissionPresets } from "../config/permissionPresets";
import { RoleLimits } from "../config/policies";
import i18n from "../i18n";

interface RoleNameInput {
  scope: RoleScope;
  permissions: readonly PermissionID[];
  catalog: PermissionDefinition[];
  existingRoles: readonly { name: string; scope: RoleScope }[];
}

export function formatRoleName({
  scope,
  permissions,
  catalog,
  existingRoles,
}: RoleNameInput): string {
  const base = suggestedRoleName(scope, permissions, catalog);
  const used = new Set(
    existingRoles
      .filter((role) => role.scope === scope)
      .map((role) => role.name.toLowerCase()),
  );
  let name = shorten(base, RoleLimits.NameLength);
  for (let number = 2; used.has(name.toLowerCase()); number++) {
    const suffix = ` ${number}`;
    name = shorten(base, RoleLimits.NameLength - suffix.length) + suffix;
  }
  return name;
}

function suggestedRoleName(
  scope: RoleScope,
  permissions: readonly PermissionID[],
  catalog: PermissionDefinition[],
): string {
  if (scope === RoleScope.Platform)
    return i18n.t(
      permissions.includes(Permission.ServersCreate)
        ? "roleNames.serverCreator"
        : "roleNames.platformMember",
    );
  if (permissions.length === 1 && permissions[0] === Permission.ServerView)
    return i18n.t("roleNames.serverViewer");
  const selected = new Set(permissions);
  if (catalog.length) {
    for (const [label, grants] of serverPermissionPresets(catalog)) {
      const expanded = expandPermissions([...grants], catalog);
      if (
        expanded.length === selected.size &&
        expanded.every((id) => selected.has(id))
      )
        return i18n.t(`permissionPresets.${label}`);
    }
  }
  const prerequisites = new Set(
    catalog
      .filter((item) => selected.has(item.id))
      .flatMap((item) => item.requires),
  );
  const labels = catalog
    .filter((item) => selected.has(item.id) && !prerequisites.has(item.id))
    .map((item) =>
      i18n.t(`permissionNames.${item.id.replaceAll(".", "_")}`, {
        defaultValue: item.label,
      }),
    );
  return labels.length
    ? `${i18n.t("common.server")} · ${labels.join(" + ")}`
    : i18n.t("roleNames.customServerRole");
}

function shorten(value: string, length: number): string {
  const characters = Array.from(value);
  return characters.length > length
    ? `${characters.slice(0, length - 1).join("")}…`
    : value;
}
