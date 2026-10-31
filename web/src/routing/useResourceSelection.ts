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
  const [page, encodedID = ""] = route.split("/");
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
    if (loaded && id && !selected) replaceCurrentRoute(page ?? "", route);
  }, [loaded, id, selected, page, route]);
  const select = useCallback(
    (next: T | null) => {
      window.location.hash =
        next && pageID
          ? (resourceHref(pageID, next.id) ?? page ?? "")
          : (page ?? "");
    },
    [page, pageID],
  );
  return { selected, select, missing: !!id && !selected };
}
