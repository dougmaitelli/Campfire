import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { stripTypeScriptTypes } from "node:module";
import test from "node:test";

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
