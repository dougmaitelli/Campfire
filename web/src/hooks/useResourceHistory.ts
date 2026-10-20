import { useEffect, useState } from "react";
import i18n from "../i18n";
import {
  readResourceHistory,
  type ResourceHistory,
} from "../api/resourceHistory";

const REFRESH_MS = 30_000;
const HISTORY_HOURS = 1;

export function useResourceHistory(serverID: string | undefined) {
  const [result, setResult] = useState<{
    id: string;
    history?: ResourceHistory;
    error?: string;
  } | null>(null);
  useEffect(() => {
    if (!serverID) return;
    const id = serverID;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function poll() {
      try {
        const history = await readResourceHistory(
          id,
          HISTORY_HOURS,
          AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
        );
        if (!controller.signal.aborted) setResult({ id, history });
      } catch (cause) {
        if (!controller.signal.aborted)
          setResult((previous) => ({
            id,
            ...(previous?.id === id && previous.history
              ? { history: previous.history }
              : {}),
            error:
              cause instanceof Error
                ? cause.message
                : i18n.t("resourceHistory.loadFailed"),
          }));
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => void poll(), REFRESH_MS);
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [serverID]);
  return result?.id === serverID ? result : null;
}
