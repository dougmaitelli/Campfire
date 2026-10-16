import { useSyncExternalStore } from "react";
import { getCurrentPage } from "./navigation";

const listeners = new Set<() => void>();
const guards = new Set<() => boolean>();
let currentRoute = window.location.hash.slice(1);

function navigate() {
  const next = window.location.hash.slice(1);
  if (next === currentRoute) return;
  for (const guard of guards) {
    if (!guard()) {
      window.history.replaceState(null, "", `#${currentRoute}`);
      return;
    }
  }
  currentRoute = next;
  for (const listener of listeners) listener();
}

function subscribe(onChange: () => void) {
  if (listeners.size === 0) window.addEventListener("hashchange", navigate);
  listeners.add(onChange);
  return () => {
    listeners.delete(onChange);
    if (listeners.size === 0)
      window.removeEventListener("hashchange", navigate);
  };
}

export function addNavigationGuard(guard: () => boolean) {
  guards.add(guard);
  return () => {
    guards.delete(guard);
  };
}

export function useCurrentPage() {
  return useSyncExternalStore(subscribe, getCurrentPage);
}

export function useCurrentRoute() {
  return useSyncExternalStore(subscribe, () => currentRoute);
}
