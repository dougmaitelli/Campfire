import i18n from "../i18n";
import { useRequestError } from "./useRequestError";
import { ConnectionError } from "../api/client";
import { useCallback, useEffect, useRef, useState } from "react";

export function useCollection<T extends { id: string | number }>(
  list: (signal: AbortSignal) => Promise<T[]>,
  refreshIntervalMs?: number,
) {
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(true);
  const [cause, setError] = useState<Error | string | null>(null);
  const { error, connectionError } = useRequestError(cause);
  const active = useRef<AbortController | null>(null);
  const load = useCallback(
    (controller: AbortController) =>
      list(controller.signal)
        .then((result) => {
          if (!controller.signal.aborted) {
            setItems(result);
            setError(null);
          }
        })
        .catch((cause: unknown) => {
          if (!controller.signal.aborted)
            setError(
              cause instanceof Error
                ? cause
                : i18n.t("useCollection.unableToLoadData"),
            );
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
          if (active.current === controller) active.current = null;
        }),
    [list],
  );
  const refresh = useCallback(async () => {
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    setLoading(true);
    await load(controller);
  }, [load]);
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function poll() {
      if (active.current && !active.current.signal.aborted) {
        if (refreshIntervalMs && refreshIntervalMs > 0)
          timer = setTimeout(() => void poll(), refreshIntervalMs);
        return;
      }
      const controller = new AbortController();
      active.current = controller;
      await load(controller);
      if (!stopped && refreshIntervalMs && refreshIntervalMs > 0)
        timer = setTimeout(() => void poll(), refreshIntervalMs);
    }
    void poll();
    return () => {
      stopped = true;
      active.current?.abort();
      clearTimeout(timer);
    };
  }, [load, refreshIntervalMs]);
  useEffect(() => {
    // One-shot lists also need a recovery path once their inline connection
    // error is replaced by the shared notice. Never retry writes or API errors.
    if (!(cause instanceof ConnectionError) || (refreshIntervalMs ?? 0) > 0)
      return;
    const timer = setTimeout(() => {
      if (!active.current) void refresh();
    }, 3000);
    return () => clearTimeout(timer);
  }, [cause, refreshIntervalMs, refresh]);
  const upsert = useCallback((item: T) => {
    active.current?.abort();
    setItems((current) =>
      current.some((entry) => entry.id === item.id)
        ? current.map((entry) => (entry.id === item.id ? item : entry))
        : [...current, item],
    );
    setLoading(false);
    setError(null);
  }, []);
  const replace = useCallback((next: T[]) => {
    active.current?.abort();
    setItems(next);
    setLoading(false);
    setError(null);
  }, []);
  return { items, loading, error, connectionError, refresh, upsert, replace };
}
