import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { unreadNotifications } from "../api/notifications";

const Context = createContext<{ unread: number | null; refresh: () => void }>({
  unread: null,
  refresh: () => {},
});

export function NotificationsProvider({ children }: { children: ReactNode }) {
  const [unread, setUnread] = useState<number | null>(null);
  const [version, setVersion] = useState(0);
  const refresh = useCallback(() => setVersion((value) => value + 1), []);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function load() {
      try {
        const value = await unreadNotifications(
          AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
        );
        if (!controller.signal.aborted) setUnread(value);
      } catch {
        if (!controller.signal.aborted) setUnread(null);
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => {
          void load();
        }, 15000);
    }
    void load();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [version]);
  return (
    <Context.Provider value={{ unread, refresh }}>{children}</Context.Provider>
  );
}
export function useNotifications() {
  return useContext(Context);
}
