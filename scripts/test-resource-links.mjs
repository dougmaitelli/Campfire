import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { stripTypeScriptTypes } from "node:module";
import test from "node:test";
import { loadWebModule } from "./helpers/load-web-module.mjs";

test("development versions use short SHAs without changing release versions", async () => {
  const { formatVersion } = await loadWebModule(
    new URL("../web/src/utils/formatVersion.ts", import.meta.url),
  );
  assert.equal(
    formatVersion("dev-e00ac7305d2a2311b8b88136c8829d8a8b3cc746"),
    "dev-e00ac730…",
  );
  for (const version of ["1.0.0", "v1.0.0-rc.1", "dev", "dev-12345678"])
    assert.equal(formatVersion(version), version);
});

test("missing-resource redirects replace history and ignore outdated requests", async (t) => {
  const replacements = [];
  const location = { hash: "#servers/deleted/console" };
  const original = Object.getOwnPropertyDescriptor(globalThis, "window");
  t.after(() => {
    if (original) Object.defineProperty(globalThis, "window", original);
    else delete globalThis.window;
  });
  globalThis.window = {
    location,
    history: {
      replaceState(_state, _title, hash) {
        replacements.push(hash);
        location.hash = hash;
      },
    },
  };
  const { replaceCurrentRoute } = await loadWebModule(
    new URL("../web/src/routing/useCurrentPage.ts", import.meta.url),
  );
  replaceCurrentRoute("servers", "servers/deleted/console");
  assert.equal(location.hash, "#servers");
  assert.deepEqual(replacements, ["#servers"]);
  location.hash = "#sparks/another";
  replaceCurrentRoute("servers", "servers/deleted/console");
  assert.equal(location.hash, "#sparks/another");
  assert.equal(replacements.length, 1);
});

const source = stripTypeScriptTypes(
  await readFile(
    new URL("../web/src/routing/resourceLinks.ts", import.meta.url),
    "utf8",
  ),
);
const { createResourceLinks } = await import(
  `data:text/javascript;base64,${Buffer.from(source).toString("base64")}`
);

test("resource links target specific records and encode resource IDs", () => {
  const links = createResourceLinks(() => true);
  assert.equal(links.server("server /#1"), "#servers/server%20%2F%231/console");
  assert.equal(links.server("one", "backups"), "#servers/one/backups");
  assert.equal(links.spark("node"), "#sparks/node");
  assert.equal(links.ember("game"), "#embers/game");
  assert.equal(links.user(12), "#users/12");
  assert.equal(links.role(3), "#roles/3");
  assert.equal(links.spark(""), undefined);
  assert.equal(links.user(0), undefined);
});

test("management references remain text without destination permissions", () => {
  const links = createResourceLinks(
    (permission) => permission === "users.manage",
  );
  assert.equal(links.user(12), "#users/12");
  assert.equal(links.spark("node"), undefined);
  assert.equal(links.ember("game"), undefined);
  assert.equal(links.role(3), undefined);
  assert.equal(links.target("spark:node"), undefined);
});

test("audit references link only recognized resource targets", () => {
  const links = createResourceLinks(() => true);
  assert.equal(links.target("server:abc"), "#servers/abc/console");
  assert.equal(links.target("schedule:once"), "#schedules/once");
  assert.equal(links.target("user:12"), "#users/12");
  assert.equal(links.target("role:3"), "#roles/3");
  for (const target of [
    "auth:login",
    "api",
    "server",
    "server:",
    "user:abc",
    "server:one:extra",
    "token:3",
  ]) {
    assert.equal(links.target(target), undefined, target);
  }
});
