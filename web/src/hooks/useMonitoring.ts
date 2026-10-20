import i18n from "../i18n";
import { useCallback, useEffect, useState } from "react";
import { readMonitoring, type MonitoringSnapshot } from "../api/monitoring";

const DEFAULT_MONITORING_INTERVAL_SECONDS = 10;

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
    let intervalSeconds = DEFAULT_MONITORING_INTERVAL_SECONDS;
    async function poll() {
      try {
        const value = await readMonitoring(
          AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
        );
        if (!controller.signal.aborted) {
          intervalSeconds = value.sample_interval_seconds;
          setSnapshot(value);
          setReceived(Date.now());
          setNow(Date.now());
          setError(null);
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("useMonitoring.unableToRefreshUsage"),
          );
      } finally {
        if (!controller.signal.aborted) setLoading(false);
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => {
          void poll();
        }, intervalSeconds * 1000);
    }
    void poll();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [revision]);
  const refresh = useCallback(() => {
    setLoading(true);
    setError(null);
    setRevision((value) => value + 1);
  }, []);
  return {
    snapshot,
    error,
    loading,
    refresh,
    elapsed: received ? Math.max(0, Math.floor((now - received) / 1000)) : 0,
  };
}
