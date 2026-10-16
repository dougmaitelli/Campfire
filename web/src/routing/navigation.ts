export const navigation = [
  { id: "dashboard", label: "Dashboard", icon: "dashboard" },
  { id: "sparks", label: "Sparks", icon: "sparks" },
  { id: "embers", label: "Embers", icon: "embers" },
  { id: "servers", label: "Servers", icon: "servers" },
  { id: "backups", label: "Backups", icon: "backups" },
  { id: "users", label: "Users", icon: "users" },
  { id: "roles", label: "Roles", icon: "roles" },
  { id: "schedules", label: "Schedules", icon: "schedules" },
  { id: "notifications", label: "Notifications", icon: "notifications" },
  { id: "audit", label: "Audit", icon: "audit" },
] as const;

export type NavigationItem = (typeof navigation)[number];
export type NavigationIcon = NavigationItem["icon"];

export function getCurrentPage(): NavigationItem {
  const id = window.location.hash.slice(1).split("/")[0];
  return navigation.find((item) => item.id === id) ?? navigation[0];
}
