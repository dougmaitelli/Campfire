import { RuntimeState } from "../contracts/domains";
import i18n from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { listServerStatuses } from "../api/runtime";
import type { RuntimeStatus } from "../api/runtime";

export function useServerRuntime(ids: string[]) {
  const key = ids.join(",");
  const [statuses, setStatuses] = useState<
    Record<string, RuntimeStatus | undefined>
  >({});
  const generation = useRef(0);
  const upsert = useCallback((status: RuntimeStatus) => {
    generation.current++;
    setStatuses((current) => ({ ...current, [status.server_id]: status }));
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function poll() {
      const started = generation.current;
      const entries: [string, RuntimeStatus][] = [];
      const list = key ? key.split(",") : [];
      if (list.length > 0) {
        try {
          const statuses = await listServerStatuses(
            AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]),
          );
          const selected = new Set(list);
          for (const status of statuses) {
            if (selected.has(status.server_id))
              entries.push([status.server_id, status]);
          }
        } catch {
          for (const id of list)
            entries.push([
              id,
              {
                server_id: id,
                state: RuntimeState.Unknown,
                ready: false,
                busy: false,
                disk_quota_enforced: false,
                progress: i18n.t("useServerRuntime.sparkOrDockerUnavailable"),
              },
            ]);
        }
      }
      if (!controller.signal.aborted) {
        if (started === generation.current)
          setStatuses(Object.fromEntries(entries));
        timer = setTimeout(() => {
          void poll();
        }, 3000);
      }
    }
    void poll();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [key]);
  return { statuses, upsert };
}
