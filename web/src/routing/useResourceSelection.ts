import { useCallback, useEffect } from "react";
import { useCurrentRoute, replaceCurrentRoute } from "./useCurrentPage";
import { resourceHref } from "./resourceLinks";
import { getRoute, type PageID } from "./routeRegistry";

// Resource drawers share their selection with the URL, including browser history.
export function useResourceSelection<T extends { id: string | number }>(
  items: T[],
  loaded: boolean,
) {
  const route = useCurrentRoute();
  const [path, search = ""] = route.split("?");
  const [page, encodedID = ""] = path!.split("/");
  const listQuery = new URLSearchParams(search);
  // Nested history pages belong to the selected resource, not its parent list.
  listQuery.delete("execution_page");
  listQuery.delete("notification_page");
  const suffix = listQuery.size ? `?${listQuery}` : "";
  const pageID = page && getRoute(page) ? (page as PageID) : undefined;
  let id = "";
  try {
    id = decodeURIComponent(encodedID);
  } catch {
    // Malformed resource IDs behave like missing resources.
    id = encodedID;
  }
  const selected = items.find((item) => String(item.id) === id) ?? null;
  useEffect(() => {
    if (loaded && id && !selected)
      replaceCurrentRoute(`${page ?? ""}${suffix}`, route);
  }, [loaded, id, selected, page, route, suffix]);
  const select = useCallback(
    (next: T | null) => {
      const path =
        next && pageID
          ? (resourceHref(pageID, next.id) ?? page ?? "")
          : (page ?? "");
      window.location.hash = `${path}${suffix}`;
    },
    [page, pageID, suffix],
  );
  return { selected, select, missing: !!id && !selected };
}
