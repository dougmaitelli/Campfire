import type { AdminResourceOperation } from "../permissions";
import { ResourceKind, ServerTab } from "../contracts/domains";
import { lazy, type ComponentType } from "react";

export type InspectionProps = { id: string; onClose: () => void };

interface ResourceDefinition {
  kind: ResourceKind;
  operation?: AdminResourceOperation;
  numericID?: boolean;
  defaultTab?: ServerTab;
  Drawer: ComponentType<InspectionProps>;
}

export interface RouteDefinition {
  Page: ComponentType;
  adminOnly: boolean;
  resource?: ResourceDefinition;
}

// Keep page rendering, access, and resource inspection together. Lazy imports
// let navigation and link helpers use the registry without loading every page.
export const routeRegistry = {
  dashboard: {
    Page: lazy(() =>
      import("../pages/DashboardPage").then((m) => ({
        default: m.DashboardPage,
      })),
    ),
    adminOnly: false,
  },
  sparks: {
    Page: lazy(() =>
      import("../pages/SparksPage").then((m) => ({ default: m.SparksPage })),
    ),
    adminOnly: true,
    resource: {
      kind: ResourceKind.Spark,
      operation: "manageSparks",
      Drawer: lazy(() =>
        import("./resourceDrawers").then((m) => ({ default: m.InspectSpark })),
      ),
    },
  },
  embers: {
    Page: lazy(() =>
      import("../pages/EmbersPage").then((m) => ({ default: m.EmbersPage })),
    ),
    adminOnly: true,
    resource: {
      kind: ResourceKind.Ember,
      operation: "manageEmbers",
      Drawer: lazy(() =>
        import("./resourceDrawers").then((m) => ({ default: m.InspectEmber })),
      ),
    },
  },
  servers: {
    Page: lazy(() =>
      import("./ServerRoute").then((m) => ({ default: m.ServerRoute })),
    ),
    adminOnly: false,
    resource: {
      kind: ResourceKind.Server,
      defaultTab: ServerTab.Console,
      Drawer: lazy(() =>
        import("./resourceDrawers").then((m) => ({ default: m.InspectServer })),
      ),
    },
  },
  backups: {
    Page: lazy(() =>
      import("../pages/BackupsPage").then((m) => ({ default: m.BackupsPage })),
    ),
    adminOnly: false,
  },
  jobs: {
    Page: lazy(() =>
      import("../pages/JobsPage").then((m) => ({ default: m.JobsPage })),
    ),
    adminOnly: true,
  },
  users: {
    Page: lazy(() =>
      import("../pages/UsersPage").then((m) => ({ default: m.UsersPage })),
    ),
    adminOnly: true,
    resource: {
      kind: ResourceKind.User,
      operation: "manageUsers",
      numericID: true,
      Drawer: lazy(() =>
        import("./resourceDrawers").then((m) => ({ default: m.InspectUser })),
      ),
    },
  },
  roles: {
    Page: lazy(() =>
      import("../pages/RolesPage").then((m) => ({ default: m.RolesPage })),
    ),
    adminOnly: true,
    resource: {
      kind: ResourceKind.Role,
      operation: "manageRoles",
      numericID: true,
      Drawer: lazy(() =>
        import("./resourceDrawers").then((m) => ({ default: m.InspectRole })),
      ),
    },
  },
  schedules: {
    Page: lazy(() =>
      import("../pages/SchedulesPage").then((m) => ({
        default: m.SchedulesPage,
      })),
    ),
    adminOnly: false,
    resource: {
      kind: ResourceKind.Schedule,
      Drawer: lazy(() =>
        import("./resourceDrawers").then((m) => ({
          default: m.InspectSchedule,
        })),
      ),
    },
  },
  notifications: {
    Page: lazy(() =>
      import("../pages/NotificationsPage").then((m) => ({
        default: m.NotificationsPage,
      })),
    ),
    adminOnly: false,
  },
  audit: {
    Page: lazy(() =>
      import("../pages/AuditPage").then((m) => ({ default: m.AuditPage })),
    ),
    adminOnly: true,
  },
} satisfies Record<string, RouteDefinition>;

export type PageID = keyof typeof routeRegistry;

export function getRoute(id: string): RouteDefinition | undefined {
  return Object.hasOwn(routeRegistry, id)
    ? routeRegistry[id as PageID]
    : undefined;
}

export function getResourceRoute(kind: ResourceKind) {
  return Object.entries(routeRegistry).find(
    ([, route]) => (route as RouteDefinition).resource?.kind === kind,
  ) as [PageID, RouteDefinition] | undefined;
}

export function parseResourceRoute(route: string) {
  const [page = "", encodedID = "", tab] = route.replace(/^#/, "").split("/");
  try {
    return { page, id: decodeURIComponent(encodedID), tab };
  } catch {
    return { page, id: "", tab };
  }
}
