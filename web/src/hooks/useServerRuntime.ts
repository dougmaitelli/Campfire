import i18n from "../i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { serverStatus } from "../api/runtime";
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
      for (let index = 0; index < list.length; index += 6) {
        if (controller.signal.aborted) return;
        await Promise.all(
          list.slice(index, index + 6).map(async (id) => {
            try {
              entries.push([
                id,
                await serverStatus(
                  id,
                  AbortSignal.any([
                    controller.signal,
                    AbortSignal.timeout(8000),
                  ]),
                ),
              ]);
            } catch {
              entries.push([
                id,
                {
                  server_id: id,
                  state: "unknown",
                  ready: false,
                  busy: false,
                  disk_quota_enforced: false,
                  progress: i18n.t("useServerRuntime.sparkOrDockerUnavailable"),
                },
              ]);
            }
          }),
        );
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
