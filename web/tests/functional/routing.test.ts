import { expect, test, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import {
  replaceCurrentRoute,
  useCurrentPage,
  useCurrentRoute,
  addNavigationGuard,
} from "../../src/routing/useCurrentPage";

test("route subscriptions catch up after navigation while unmounted", () => {
  replaceCurrentRoute("servers/survival/console");
  const previous = renderHook(useCurrentRoute);
  expect(previous.result.current).toBe("servers/survival/console");
  previous.unmount();

  window.history.replaceState(null, "", "#servers");
  const current = renderHook(useCurrentRoute);
  expect(current.result.current).toBe("servers");
});

test("page and resource routes share the same accepted navigation snapshot", () => {
  replaceCurrentRoute("servers/survival/console");
  const route = renderHook(() => ({
    page: useCurrentPage(),
    path: useCurrentRoute(),
  }));
  act(() => {
    window.history.replaceState(null, "", "#schedules");
    route.rerender();
  });
  expect(route.result.current.page.id).toBe("servers");
  expect(route.result.current.path).toBe("servers/survival/console");

  act(() => window.dispatchEvent(new HashChangeEvent("hashchange")));
  expect(route.result.current.page.id).toBe("schedules");
  expect(route.result.current.path).toBe("schedules");
});

test("missing-resource redirects replace history and ignore outdated requests", () => {
  window.location.hash = "#servers/deleted/console";
  const replace = vi.spyOn(window.history, "replaceState");
  replaceCurrentRoute("servers", "servers/deleted/console");
  expect(window.location.hash).toBe("#servers");
  expect(replace).toHaveBeenCalledTimes(1);
  window.location.hash = "#sparks/another";
  replaceCurrentRoute("servers", "servers/deleted/console");
  expect(window.location.hash).toBe("#sparks/another");
  expect(replace).toHaveBeenCalledTimes(1);
});

test("blocked navigation keeps the URL, page, and resource route together", () => {
  replaceCurrentRoute("servers/survival/files");
  const route = renderHook(() => ({
    page: useCurrentPage(),
    path: useCurrentRoute(),
  }));
  const removeGuard = addNavigationGuard(() => false);
  try {
    act(() => {
      window.history.replaceState(null, "", "#schedules");
      window.dispatchEvent(new HashChangeEvent("hashchange"));
    });
    expect(window.location.hash).toBe("#servers/survival/files");
    expect(route.result.current.page.id).toBe("servers");
    expect(route.result.current.path).toBe("servers/survival/files");
  } finally {
    removeGuard();
  }
  act(() => {
    window.history.replaceState(null, "", "#servers");
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  });
  expect(route.result.current.path).toBe("servers");
});
