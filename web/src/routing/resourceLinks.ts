import { getResourceRoute } from "./routeRegistry";

export function resourceHref(page: string, id: string | number) {
  return id ? `#${page}/${encodeURIComponent(id)}` : undefined;
}

export function createResourceLinks(
  canManage: (permission: string) => boolean,
) {
  function link(kind: string, id: string | number, tab?: string) {
    const match = getResourceRoute(kind);
    if (!match) return undefined;
    const [page, route] = match;
    const resource = route.resource!;
    if (resource.permission && !canManage(resource.permission))
      return undefined;
    if (resource.numericID) {
      if (!/^\d+$/.test(String(id))) return undefined;
      id = Number(id);
    }
    const href = resourceHref(page, id);
    const activeTab = tab ?? resource.defaultTab;
    return href && activeTab ? `${href}/${activeTab}` : href;
  }
  return {
    server: (id: string, tab?: string) => link("server", id, tab),
    spark: (id: string) => link("spark", id),
    ember: (id: string) => link("ember", id),
    user: (id: number) => link("user", id),
    role: (id: number) => link("role", id),
    schedule: (id: string) => link("schedule", id),
    target: (target: string) => {
      const [kind, id, extra] = target.split(":");
      return kind && id && extra === undefined ? link(kind, id) : undefined;
    },
  };
}
