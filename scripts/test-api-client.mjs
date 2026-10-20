import assert from "node:assert/strict";
import test from "node:test";
import { loadWebModule } from "./helpers/load-web-module.mjs";

const sourceURL = new URL("../web/src/api/client.ts", import.meta.url);
const { apiFetch, readJSON, APIError } = await loadWebModule(sourceURL);
const { readResourceHistory } = await loadWebModule(
  new URL("../web/src/api/resourceHistory.ts", import.meta.url),
);

test("API failures preserve HTTP status for missing-resource navigation", async () => {
  for (const status of [404, 403, 500]) {
    await assert.rejects(
      readJSON(Response.json({ error: "Resource unavailable" }, { status })),
      (error) => error instanceof APIError && error.status === status,
    );
  }
});

test("history requests use bounded ranges and preserve missing measurements", async (t) => {
  const usage = {
    server_id: "one",
    state: "sampled",
    cpu_percent: null,
    memory_bytes: 0,
    disk_bytes: null,
    sampled_at: 0,
    disk_sampled_at: 0,
    sample_age_seconds: 0,
    disk_age_seconds: 0,
    container_error: "",
    disk_error: "",
    disk_quota_enforced: false,
    disk_quota_error: "",
  };
  const history = {
    from: 100,
    to: 400,
    interval_seconds: 300,
    samples: [{ collected_at: 100, usage }],
  };
  t.mock.method(globalThis, "fetch", async (url) => {
    const parsed = new URL(url, "https://panel.test");
    assert.equal(parsed.pathname, "/api/v1/servers/one%2Ftwo/metrics/history");
    assert.equal(parsed.searchParams.get("limit"), "240");
    assert.equal(
      Number(parsed.searchParams.get("to")) -
        Number(parsed.searchParams.get("from")),
      86400,
    );
    return Response.json(history);
  });
  const result = await readResourceHistory(
    "one/two",
    24,
    new AbortController().signal,
  );
  assert.equal(result.samples[0].usage.cpu_percent, null);
  assert.equal(result.samples[0].usage.memory_bytes, 0);
  history.samples[0].usage.cpu_percent = -1;
  await assert.rejects(
    readResourceHistory("one/two", 24, new AbortController().signal),
    /Unexpected resource history response/,
  );
});

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
