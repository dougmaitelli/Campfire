import { expect, test } from "vitest";
import { formatVersion } from "../../src/utils/formatVersion";
import { createResourceLinks } from "../../src/routing/resourceLinks";
import {
  getRoute,
  getResourceRoute,
  parseResourceRoute,
} from "../../src/routing/routeRegistry";

test("development versions shorten SHAs without changing release versions", () => {
  expect(formatVersion("dev-e00ac7305d2a2311b8b88136c8829d8a8b3cc746")).toBe(
    "dev-e00ac730…",
  );
  for (const version of ["1.0.0", "v1.0.0-rc.1", "dev", "dev-12345678"])
    expect(formatVersion(version)).toBe(version);
});

test("registry resolves page access and resource inspection destinations", () => {
  expect(getRoute("dashboard")?.adminOnly).toBe(false);
  expect(getRoute("users")?.adminOnly).toBe(true);
  expect(getResourceRoute("user")?.[0]).toBe("users");
  expect(getResourceRoute("user")?.[1].resource?.permission).toBe(
    "users.manage",
  );
  expect(getResourceRoute("server")?.[1].resource?.Drawer).toBeDefined();
  for (const unknown of ["missing", "constructor", "toString", "__proto__"]) {
    expect(getRoute(unknown)).toBeUndefined();
    expect(getResourceRoute(unknown)).toBeUndefined();
  }
});

test("routes decode IDs and tolerate malformed URLs", () => {
  expect(parseResourceRoute("#servers/server%20%2F%231/backups")).toEqual({
    page: "servers",
    id: "server /#1",
    tab: "backups",
  });
  expect(parseResourceRoute("servers/%zz/console")).toEqual({
    page: "servers",
    id: "",
    tab: "console",
  });
  expect(parseResourceRoute("dashboard")).toEqual({
    page: "dashboard",
    id: "",
    tab: undefined,
  });
});

test("links encode IDs and honor destination permissions", () => {
  const links = createResourceLinks(() => true);
  expect(links.server("server /#1")).toBe("#servers/server%20%2F%231/console");
  expect(links.server("one", "backups")).toBe("#servers/one/backups");
  expect(links.spark("node")).toBe("#sparks/node");
  expect(links.ember("game")).toBe("#embers/game");
  expect(links.user(12)).toBe("#users/12");
  expect(links.role(3)).toBe("#roles/3");
  expect(links.spark("")).toBeUndefined();
  expect(links.user(0)).toBeUndefined();
  const restricted = createResourceLinks(
    (permission) => permission === "users.manage",
  );
  expect(restricted.user(12)).toBe("#users/12");
  expect(restricted.spark("node")).toBeUndefined();
  expect(restricted.ember("game")).toBeUndefined();
  expect(restricted.role(3)).toBeUndefined();
  expect(restricted.target("spark:node")).toBeUndefined();
});

test("audit links recognize only supported resource targets", () => {
  const links = createResourceLinks(() => true);
  expect(links.target("server:abc")).toBe("#servers/abc/console");
  expect(links.target("schedule:once")).toBe("#schedules/once");
  expect(links.target("user:12")).toBe("#users/12");
  expect(links.target("role:3")).toBe("#roles/3");
  for (const target of [
    "auth:login",
    "api",
    "server",
    "server:",
    "user:abc",
    "server:one:extra",
    "token:3",
  ])
    expect(links.target(target)).toBeUndefined();
});
