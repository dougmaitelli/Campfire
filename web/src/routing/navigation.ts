export const navigation = [
  { id: "dashboard", label: "Dashboard", icon: "dashboard" },
  { id: "sparks", label: "Sparks", icon: "sparks" },
  { id: "embers", label: "Embers", icon: "embers" },
  { id: "servers", label: "Servers", icon: "servers" },
  { id: "users", label: "Users", icon: "users" },
  { id: "roles", label: "Roles", icon: "roles" },
  { id: "schedules", label: "Schedules", icon: "schedules" },
] as const;

export type NavigationItem = (typeof navigation)[number];
export type NavigationIcon = NavigationItem["icon"];

export function getCurrentPage(): NavigationItem {
  const id = window.location.hash.slice(1);
  return navigation.find((item) => item.id === id) ?? navigation[0];
}
