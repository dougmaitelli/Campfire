import { expect, test, vi } from "vitest";
import { APIError, apiFetch, readJSON } from "../../src/api/client";
import { readResourceHistory } from "../../src/api/resourceHistory";
import type { ResourceHistory } from "../../src/api/resourceHistory";

test.each([404, 403, 500])(
  "preserves HTTP status %i for missing-resource navigation",
  async (status) => {
    const response = readJSON(
      Response.json({ error: "Resource unavailable" }, { status }),
    );
    await expect(response).rejects.toBeInstanceOf(APIError);
    await expect(response).rejects.toMatchObject({ status, name: "APIError" });
  },
);

test("history requests use bounded ranges and preserve missing measurements", async () => {
  const history: ResourceHistory = {
    from: 100,
    to: 400,
    interval_seconds: 300,
    samples: [
      {
        collected_at: 100,
        usage: {
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
        },
      },
    ],
  };
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
    const parsed = new URL(String(input), "https://panel.test");
    expect(parsed.pathname).toBe("/api/v1/servers/one%2Ftwo/metrics/history");
    expect(parsed.searchParams.get("limit")).toBe("240");
    expect(
      Number(parsed.searchParams.get("to")) -
        Number(parsed.searchParams.get("from")),
    ).toBe(86400);
    return Response.json(history);
  });
  const result = await readResourceHistory(
    "one/two",
    24,
    new AbortController().signal,
  );
  expect(result.samples[0]?.usage.cpu_percent).toBeNull();
  expect(result.samples[0]?.usage.memory_bytes).toBe(0);
  history.samples[0]!.usage.cpu_percent = -1;
  await expect(
    readResourceHistory("one/two", 24, new AbortController().signal),
  ).rejects.toThrow("Unexpected resource history response");
});

test("preserves JSON responses and backend error messages", async () => {
  await expect(
    readJSON(Response.json({ token: "one-time-token" })),
  ).resolves.toEqual({ token: "one-time-token" });
  await expect(
    readJSON(
      Response.json(
        { error: "the panel agent HTTPS listener is not configured" },
        { status: 503 },
      ),
    ),
  ).rejects.toThrow("The panel agent HTTPS listener is not configured.");
});

test.each([
  [502, "text/html", "<html>private proxy response</html>"],
  [404, "text/plain", "private backend response"],
  [201, "application/json", "invalid private token response"],
  [200, "text/html", "<html>private login page</html>"],
] as const)(
  "non-JSON HTTP %i responses identify the endpoint without leaking content",
  async (status, contentType, body) => {
    const response = new Response(body, {
      status,
      headers: { "Content-Type": contentType },
    });
    Object.defineProperty(response, "url", {
      value: "https://panel.test/api/v1/sparks/node/enrollment?secret=hidden",
    });
    const failure: unknown = await readJSON(response).catch(
      (error: unknown) => error,
    );
    expect(failure).toBeInstanceOf(Error);
    expect((failure as Error).message).toContain(
      "/api/v1/sparks/node/enrollment",
    );
    expect((failure as Error).message).toContain(
      `HTTP ${status}, ${contentType}`,
    );
    expect((failure as Error).message).not.toMatch(/private|hidden/);
  },
);

test("retains transport failures", async () => {
  const failure = new TypeError("connection interrupted");
  const response = Response.json({});
  vi.spyOn(response, "json").mockRejectedValue(failure);
  await expect(readJSON(response)).rejects.toBe(failure);
});

test("requests advertise JSON while preserving explicit Accept headers", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(Response.json({}));
  await apiFetch("/api/v1/sparks/node/enrollment", { method: "POST" });
  await apiFetch("/api/v1/sparks", {
    headers: { Accept: "application/problem+json" },
  });
  expect(new Headers(fetch.mock.calls[0]?.[1]?.headers).get("Accept")).toBe(
    "application/json",
  );
  expect(new Headers(fetch.mock.calls[1]?.[1]?.headers).get("Accept")).toBe(
    "application/problem+json",
  );
});
