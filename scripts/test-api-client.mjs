import assert from "node:assert/strict";
import test from "node:test";
import { loadWebModule } from "./helpers/load-web-module.mjs";

const sourceURL = new URL("../web/src/api/client.ts", import.meta.url);
const { apiFetch, readJSON } = await loadWebModule(sourceURL);

test("API client preserves JSON responses and backend error messages", async () => {
  assert.deepEqual(await readJSON(Response.json({ token: "one-time-token" })), {
    token: "one-time-token",
  });
  await assert.rejects(
    readJSON(
      Response.json(
        { error: "the panel agent HTTPS listener is not configured" },
        { status: 503 },
      ),
    ),
    /The panel agent HTTPS listener is not configured\./,
  );
});

test("non-JSON responses identify endpoint and HTTP status without leaking content", async () => {
  for (const [status, contentType, body] of [
    [502, "text/html", "<html>private proxy response</html>"],
    [404, "text/plain", "private backend response"],
    [201, "application/json", "invalid private token response"],
    [200, "text/html", "<html>private login page</html>"],
  ]) {
    const response = new Response(body, {
      status,
      headers: { "Content-Type": contentType },
    });
    Object.defineProperty(response, "url", {
      value: "https://panel.test/api/v1/sparks/node/enrollment?secret=hidden",
    });
    await assert.rejects(readJSON(response), (error) => {
      assert.match(error.message, /\/api\/v1\/sparks\/node\/enrollment/);
      assert.ok(error.message.includes(`HTTP ${status}, ${contentType}`));
      assert.ok(!error.message.includes("private"));
      assert.ok(!error.message.includes("hidden"));
      return true;
    });
  }
});

test("API client retains transport failures", async () => {
  const failure = new TypeError("connection interrupted");
  await assert.rejects(
    readJSON({
      json: async () => {
        throw failure;
      },
    }),
    (error) => error === failure,
  );
});

test("API requests advertise JSON while preserving explicit Accept headers", async (t) => {
  const requests = [];
  t.mock.method(globalThis, "fetch", async (_url, init) => {
    requests.push(init);
    return Response.json({});
  });
  await apiFetch("/api/v1/sparks/node/enrollment", { method: "POST" });
  await apiFetch("/api/v1/sparks", {
    headers: { Accept: "application/problem+json" },
  });
  assert.equal(requests[0].headers.get("Accept"), "application/json");
  assert.equal(requests[1].headers.get("Accept"), "application/problem+json");
});
