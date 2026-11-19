import { expect, test, vi } from "vitest";
import { Permission } from "../../src/contracts/permissions";
import {
  canPerformPlatform,
  canPerformServer,
  canPerformServerAction,
} from "../../src/permissions";
import { serverPermissionPresets } from "../../src/config/permissionPresets";
import { ServerAction } from "../../src/contracts/domains";

// The shared test setup initializes i18n; clear that module cache so the
// isolation checks also catch a direct translation dependency.
vi.hoisted(() => vi.resetModules());

// These helpers are pure policy. Type-only API imports are fine, but runtime
// imports must not initialize transport, translation, or DOM dependencies.
vi.mock("../../src/api/permissions", () => {
  throw new Error("Permission policy must not load the permission API module.");
});
vi.mock("../../src/api/client", () => {
  throw new Error("Permission policy must not load HTTP transport.");
});
vi.mock("../../src/i18n", () => {
  throw new Error("Permission policy must not initialize translations.");
});

test("permission decisions remain independent of API and UI initialization", () => {
  expect(
    canPerformPlatform(
      { permissions: [Permission.ServersCreate] },
      "createServers",
    ),
  ).toBe(true);
  const viewer = { permissions: [Permission.ServerView] };
  expect(canPerformServer(viewer, "view")).toBe(true);
  expect(canPerformServer(viewer, "operate", { admin: true })).toBe(false);
  expect(
    canPerformServerAction(
      { permissions: [Permission.ServerView, Permission.ServerOperate] },
      ServerAction.Start,
    ),
  ).toBe(true);
  expect(canPerformServerAction(viewer, ServerAction.Start)).toBe(false);
});

test("permission presets consume catalog data without loading its API module", () => {
  const presets = serverPermissionPresets([
    { id: Permission.ServerView, group: "Server", label: "View", requires: [] },
    {
      id: Permission.ServerDelete,
      group: "Server",
      label: "Delete",
      requires: [],
    },
  ]);
  expect(presets.at(-1)?.[1]).toEqual([Permission.ServerView]);
});
