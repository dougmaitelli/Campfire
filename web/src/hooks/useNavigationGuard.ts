import { useEffect } from "react";
import { addNavigationGuard } from "../routing/useCurrentPage";

export function useNavigationGuard(
  active: boolean,
  busy: boolean,
  dirty: boolean,
  confirmDiscard?: () => Promise<boolean>,
) {
  useEffect(() => {
    if (!active || (!busy && !dirty)) return;
    const remove = addNavigationGuard(() =>
      busy ? false : (confirmDiscard?.() ?? false),
    );
    function unload(event: BeforeUnloadEvent) {
      event.preventDefault();
    }
    window.addEventListener("beforeunload", unload);
    return () => {
      remove();
      window.removeEventListener("beforeunload", unload);
    };
  }, [active, busy, dirty, confirmDiscard]);
}
