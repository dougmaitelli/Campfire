import { useCallback, useEffect, useRef, useState } from "react";
import { listSparks } from "../api/sparks";
import type { Spark } from "../api/sparks";

export function useSparks() {
  const [sparks, setSparks] = useState<Spark[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const activeRequest = useRef<AbortController | null>(null);

  const load = useCallback((controller: AbortController) => {
    return listSparks(controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) {
          setSparks(result);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) {
          setError(
            cause instanceof Error ? cause.message : "Unable to load Sparks.",
          );
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
  }, []);

  const refresh = useCallback(async () => {
    activeRequest.current?.abort();
    const controller = new AbortController();
    activeRequest.current = controller;
    setLoading(true);
    setError(null);
    await load(controller);
  }, [load]);

  useEffect(() => {
    const controller = new AbortController();
    activeRequest.current = controller;
    void load(controller);
    return () => activeRequest.current?.abort();
  }, [load]);

  const addSpark = useCallback((spark: Spark) => {
    activeRequest.current?.abort();
    setSparks((current) =>
      current.some((item) => item.id === spark.id)
        ? current.map((item) => (item.id === spark.id ? spark : item))
        : [...current, spark],
    );
    setLoading(false);
    setError(null);
  }, []);

  return { sparks, loading, error, refresh, addSpark };
}
