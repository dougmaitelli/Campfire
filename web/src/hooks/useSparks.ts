import i18n from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { listSparks } from "../api/sparks";
import type { Spark } from "../api/sparks";

const SPARK_POLL_INTERVAL_MS = 2000;
const SPARK_REQUEST_TIMEOUT_MS = 10000;

export function useSparks() {
  const [sparks, setSparks] = useState<Spark[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const activeRequest = useRef<AbortController | null>(null);

  const load = useCallback((controller: AbortController) => {
    return listSparks(
      AbortSignal.any([
        controller.signal,
        AbortSignal.timeout(SPARK_REQUEST_TIMEOUT_MS),
      ]),
    )
      .then((result) => {
        if (!controller.signal.aborted) {
          setSparks(result);
          setError(null);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted) {
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("useSparks.unableToLoadSparks"),
          );
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
        if (activeRequest.current === controller) activeRequest.current = null;
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
      if (activeRequest.current) return;
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
    activeRequest.current = null;
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
