import { expect, test, vi } from "vitest";
import { replaceCurrentRoute } from "../../src/routing/useCurrentPage";

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
