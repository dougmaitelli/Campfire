import { useSyncExternalStore } from "react";
import { getCurrentPage } from "./navigation";

function subscribe(onChange: () => void) {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
}

export function useCurrentPage() {
  return useSyncExternalStore(subscribe, getCurrentPage);
}
