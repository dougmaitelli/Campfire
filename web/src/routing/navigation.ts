import i18n from "../i18n";
import { routeRegistry, type PageID } from "./routeRegistry";

export const navigation = (Object.keys(routeRegistry) as PageID[]).map(
  (id) => ({
    id,
    get label() {
      return i18n.t(`common.${id}`);
    },
    icon: id,
  }),
);

export type NavigationItem = (typeof navigation)[number];
export type NavigationIcon = NavigationItem["icon"];

export function getCurrentPage(): NavigationItem {
  const id = window.location.hash.slice(1).split("/")[0];
  return navigation.find((item) => item.id === id) ?? navigation[0]!;
}
