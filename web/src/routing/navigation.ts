import i18n from "../i18n";
export const navigation = [
  {
    id: "dashboard",
    get label() {
      return i18n.t("common.dashboard");
    },
    icon: "dashboard",
  },
  {
    id: "sparks",
    get label() {
      return i18n.t("common.sparks");
    },
    icon: "sparks",
  },
  {
    id: "embers",
    get label() {
      return i18n.t("common.embers");
    },
    icon: "embers",
  },
  {
    id: "servers",
    get label() {
      return i18n.t("common.servers");
    },
    icon: "servers",
  },
  {
    id: "backups",
    get label() {
      return i18n.t("common.backups");
    },
    icon: "backups",
  },
  {
    id: "users",
    get label() {
      return i18n.t("common.users");
    },
    icon: "users",
  },
  {
    id: "roles",
    get label() {
      return i18n.t("common.roles");
    },
    icon: "roles",
  },
  {
    id: "schedules",
    get label() {
      return i18n.t("common.schedules");
    },
    icon: "schedules",
  },
  {
    id: "notifications",
    get label() {
      return i18n.t("common.notifications");
    },
    icon: "notifications",
  },
  {
    id: "audit",
    get label() {
      return i18n.t("common.audit");
    },
    icon: "audit",
  },
] as const;

export type NavigationItem = (typeof navigation)[number];
export type NavigationIcon = NavigationItem["icon"];

export function getCurrentPage(): NavigationItem {
  const id = window.location.hash.slice(1).split("/")[0];
  return navigation.find((item) => item.id === id) ?? navigation[0];
}
