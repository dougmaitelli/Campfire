import { useCallback, useEffect, useRef } from "react";
import type { Page } from "../api/pagination";
import { API_READ_TIMEOUT_MS } from "../api/client";
import { useCollection } from "./useCollection";

// One request per displayed page. useCollection aborts superseded loads and
// serializes polling; identity prevents showing rows from an old page/filter.
export function usePagedCollection<T>(
  load: (signal: AbortSignal, page: number) => Promise<Page<T>>,
  page: number,
  refreshIntervalMs = 10000,
) {
  const fetch = useCallback(
    async (signal: AbortSignal) => [
      {
        id: page,
        loader: load,
        ...(await load(
          AbortSignal.any([signal, AbortSignal.timeout(API_READ_TIMEOUT_MS)]),
          page,
        )),
      },
    ],
    [load, page],
  );
  const collection = useCollection(fetch, refreshIntervalMs);
  const currentRefresh = useRef<typeof collection.refresh | null>(null);
  useEffect(() => {
    currentRefresh.current = collection.refresh;
    return () => {
      currentRefresh.current = null;
    };
  }, [collection.refresh]);
  // Actions can finish after navigation. Refresh the current page, not the
  // page captured when the action started, and do nothing after unmount.
  const refresh = useCallback(async () => {
    await currentRefresh.current?.();
  }, []);
  const result = collection.items.find(
    (item) => item.id === page && item.loader === load,
  );
  return {
    items: result?.items ?? [],
    pagination: result?.pagination,
    loading:
      collection.loading ||
      (!result && !collection.error && !collection.connectionError),
    error: collection.error,
    refresh,
  };
}
