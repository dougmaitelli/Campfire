import { useSyncExternalStore } from "react";
import { getCurrentPage } from "./navigation";

const listeners = new Set<() => void>();
const guards = new Set<() => boolean | Promise<boolean>>();
let currentRoute = window.location.hash.slice(1);
let navigationID = 0;

async function navigate() {
  const id = ++navigationID;
  const next = window.location.hash.slice(1);
  if (next === currentRoute) return;
  for (const guard of guards) {
    const decision = guard();
    const allowed = typeof decision === "boolean" ? decision : await decision;
    if (id !== navigationID) return;
    if (!allowed) {
      window.history.replaceState(null, "", `#${currentRoute}`);
      return;
    }
  }
  currentRoute = next;
  for (const listener of listeners) listener();
}

function onHashChange() {
  void navigate();
}

function subscribe(onChange: () => void) {
  if (listeners.size === 0) {
    currentRoute = window.location.hash.slice(1);
    window.addEventListener("hashchange", onHashChange);
  }
  listeners.add(onChange);
  return () => {
    listeners.delete(onChange);
    if (listeners.size === 0) {
      navigationID++;
      window.removeEventListener("hashchange", onHashChange);
    }
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
  void navigate();
}

export function addNavigationGuard(guard: () => boolean | Promise<boolean>) {
  guards.add(guard);
  return () => {
    guards.delete(guard);
  };
}

export function useCurrentPage() {
  return getCurrentPage(useCurrentRoute());
}

export function useCurrentRoute() {
  return useSyncExternalStore(subscribe, () => currentRoute);
}
