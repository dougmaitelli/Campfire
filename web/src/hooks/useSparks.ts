import { useCallback, useEffect, useRef, useState } from "react";
import { listSparks } from "../api/sparks";
import type { Spark } from "../api/sparks";

const SPARK_POLL_INTERVAL_MS = 30000;

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
    const timer = window.setInterval(() => {
      activeRequest.current?.abort();
      const poll = new AbortController();
      activeRequest.current = poll;
      void load(poll);
    }, SPARK_POLL_INTERVAL_MS);
    return () => {
      window.clearInterval(timer);
      activeRequest.current?.abort();
    };
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
