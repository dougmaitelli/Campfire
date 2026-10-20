export function resourceHref(page: string, id: string | number) {
  return id ? `#${page}/${encodeURIComponent(id)}` : undefined;
}

export function createResourceLinks(
  canManage: (permission: string) => boolean,
) {
  const management = (page: string, permission: string, id: string | number) =>
    canManage(permission) ? resourceHref(page, id) : undefined;
  const links = {
    server: (id: string, tab = "console") => {
      const href = resourceHref("servers", id);
      return href ? `${href}/${tab}` : undefined;
    },
    spark: (id: string) => management("sparks", "sparks.manage", id),
    ember: (id: string) => management("embers", "embers.manage", id),
    user: (id: number) => management("users", "users.manage", id),
    role: (id: number) => management("roles", "roles.manage", id),
    schedule: (id: string) => resourceHref("schedules", id),
  };
  return {
    ...links,
    target: (target: string) => {
      const [kind, id, extra] = target.split(":");
      if (!id || extra !== undefined) return undefined;
      switch (kind) {
        case "server":
          return links.server(id);
        case "spark":
          return links.spark(id);
        case "ember":
          return links.ember(id);
        case "user":
          return /^\d+$/.test(id) ? links.user(Number(id)) : undefined;
        case "role":
          return /^\d+$/.test(id) ? links.role(Number(id)) : undefined;
        case "schedule":
          return links.schedule(id);
        default:
          return undefined;
      }
    },
  };
}
