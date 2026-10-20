import { useCallback } from "react";
import { useCurrentRoute } from "./useCurrentPage";
import { resourceHref } from "./resourceLinks";

// Resource drawers share their selection with the URL, including browser history.
export function useResourceSelection<T extends { id: string | number }>(
  items: T[],
) {
  const route = useCurrentRoute();
  const [page, encodedID = ""] = route.split("/");
  let id = "";
  try {
    id = decodeURIComponent(encodedID);
  } catch {
    // Malformed resource IDs behave like missing resources.
    id = encodedID;
  }
  const selected = items.find((item) => String(item.id) === id) ?? null;
  const select = useCallback(
    (next: T | null) => {
      window.location.hash = next
        ? (resourceHref(page ?? "", next.id) ?? page ?? "")
        : (page ?? "");
    },
    [page],
  );
  return { selected, select, missing: !!id && !selected };
}
