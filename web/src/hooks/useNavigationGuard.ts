import i18n from "../i18n";
import { useEffect } from "react";
import { addNavigationGuard } from "../routing/useCurrentPage";

export function useNavigationGuard(
  active: boolean,
  busy: boolean,
  dirty: boolean,
) {
  useEffect(() => {
    if (!active || (!busy && !dirty)) return;
    const remove = addNavigationGuard(
      () =>
        !busy &&
        window.confirm(i18n.t("useNavigationGuard.discardUnsavedChanges")),
    );
    function unload(event: BeforeUnloadEvent) {
      event.preventDefault();
    }
    window.addEventListener("beforeunload", unload);
    return () => {
      remove();
      window.removeEventListener("beforeunload", unload);
    };
  }, [active, busy, dirty]);
}
