import { useCallback } from "react";
import { useCurrentRoute } from "../routing/useCurrentPage";

// Keep page/filter state in the hash router so Back/Forward restores a page.
// Nested lists use their own parameter rather than changing their parent's page.
export function usePagination(parameter = "page") {
  const route = useCurrentRoute();
  const query = new URLSearchParams(route.split("?")[1] ?? "");
  const value = Number(query.get(parameter) ?? 1);
  const page =
    Number.isSafeInteger(value) && value > 0 && value <= 21474836 ? value : 1;
  const update = useCallback((changes: Record<string, string | null>) => {
    const current = window.location.hash.slice(1);
    const [path, search] = current.split("?");
    const next = new URLSearchParams(search);
    for (const [key, value] of Object.entries(changes)) {
      if (value === null || value === "") next.delete(key);
      else next.set(key, value);
    }
    window.location.hash = `${path}${next.size ? `?${next}` : ""}`;
  }, []);
  const setPage = useCallback(
    (next: number) => update({ [parameter]: next === 1 ? null : String(next) }),
    [parameter, update],
  );
  return { page, setPage, query, update };
}
