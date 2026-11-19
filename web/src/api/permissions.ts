import { isEnumValue } from "../utils/validation";
import { apiFetch, readJSON } from "./client";

import { Permission } from "../contracts/permissions";
export { Permission } from "../contracts/permissions";
export type PermissionID = Permission;

export function isPermissionID(value: unknown): value is PermissionID {
  return isEnumValue(Permission, value);
}

export interface PermissionDefinition {
  id: PermissionID;
  group: string;
  label: string;
  requires: PermissionID[];
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
