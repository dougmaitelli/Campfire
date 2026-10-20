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

// Replace stale destinations so Back does not reopen the missing record.
export function replaceCurrentRoute(route: string, expectedRoute?: string) {
  if (
    expectedRoute !== undefined &&
    window.location.hash.slice(1) !== expectedRoute
  )
    return;
  window.history.replaceState(null, "", `#${route}`);
  navigate();
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
