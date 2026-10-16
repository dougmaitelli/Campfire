import { useCallback, useEffect, useState } from "react";
import { readMonitoring, type MonitoringSnapshot } from "../api/monitoring";

export function useMonitoring() {
  const [snapshot, setSnapshot] = useState<MonitoringSnapshot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const [received, setReceived] = useState(0);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function poll() {
      try {
        const value = await readMonitoring(
          AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
        );
        if (!controller.signal.aborted) {
          setSnapshot(value);
          setReceived(Date.now());
          setNow(Date.now());
          setError(null);
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : "Unable to refresh usage.",
          );
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => {
          void poll();
        }, 1000);
    }
    void poll();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [revision]);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  return {
    snapshot,
    error,
    loading,
    refresh,
    elapsed: received ? Math.max(0, Math.floor((now - received) / 1000)) : 0,
  };
}
