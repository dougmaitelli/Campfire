import { getResourceRoute, type PageID } from "./routeRegistry";
import type { AdminResourceOperation } from "../permissions";
import { ResourceKind, type ServerTab } from "../contracts/domains";
import { isEnumValue } from "../utils/validation";

export function resourceHref(page: PageID, id: string | number) {
  return id ? `#${page}/${encodeURIComponent(id)}` : undefined;
}

export function createResourceLinks(
  canManage: (operation: AdminResourceOperation) => boolean,
) {
  function link(kind: ResourceKind, id: string | number, tab?: ServerTab) {
    const match = getResourceRoute(kind);
    if (!match) return undefined;
    const [page, route] = match;
    const resource = route.resource!;
    if (resource.operation && !canManage(resource.operation)) return undefined;
    if (resource.numericID) {
      if (!/^\d+$/.test(String(id))) return undefined;
      id = Number(id);
    }
    const href = resourceHref(page, id);
    const activeTab = tab ?? resource.defaultTab;
    return href && activeTab ? `${href}/${activeTab}` : href;
  }
  return {
    server: (id: string, tab?: ServerTab) => link(ResourceKind.Server, id, tab),
    spark: (id: string) => link(ResourceKind.Spark, id),
    ember: (id: string) => link(ResourceKind.Ember, id),
    user: (id: number) => link(ResourceKind.User, id),
    role: (id: number) => link(ResourceKind.Role, id),
    schedule: (id: string) => link(ResourceKind.Schedule, id),
    target: (target: string) => {
      const [kind, id, extra] = target.split(":");
      return isEnumValue(ResourceKind, kind) && id && extra === undefined
        ? link(kind, id)
        : undefined;
    },
  };
}
