import i18n from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";

export function useCollection<T extends { id: string | number }>(
  list: (signal: AbortSignal) => Promise<T[]>,
) {
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const active = useRef<AbortController | null>(null);
  const load = useCallback(
    (controller: AbortController) =>
      list(controller.signal)
        .then((result) => {
          if (!controller.signal.aborted) setItems(result);
        })
        .catch((cause: unknown) => {
          if (!controller.signal.aborted)
            setError(
              cause instanceof Error
                ? cause.message
                : i18n.t("useCollection.unableToLoadData"),
            );
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        }),
    [list],
  );
  const refresh = useCallback(async () => {
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    setLoading(true);
    setError(null);
    await load(controller);
  }, [load]);
  useEffect(() => {
    const controller = new AbortController();
    active.current = controller;
    void load(controller);
    return () => active.current?.abort();
  }, [load]);
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
  return { items, loading, error, refresh, upsert, replace };
}
