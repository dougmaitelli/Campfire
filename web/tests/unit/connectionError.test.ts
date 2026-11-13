import { afterEach, expect, test, vi } from "vitest";
import {
  APIError,
  ConnectionError,
  apiFetch,
  readJSON,
} from "../../src/api/client";

afterEach(() => vi.restoreAllMocks());

test.each([
  new TypeError("Failed to fetch"),
  new DOMException("Network request failed", "NetworkError"),
  new DOMException("Request timed out", "TimeoutError"),
])(
  "transport failures are classified without matching browser messages: %s",
  async (cause) => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(cause);
    await expect(apiFetch("/api/v1/monitoring")).rejects.toBeInstanceOf(
      ConnectionError,
    );
    await expect(apiFetch("/api/v1/monitoring")).rejects.toHaveProperty(
      "cause",
      cause,
    );
  },
);

test("cancellation is not reported as a lost connection", async () => {
  const cause = new DOMException("The request was aborted", "AbortError");
  vi.spyOn(globalThis, "fetch").mockRejectedValue(cause);
  await expect(apiFetch("/api/v1/monitoring")).rejects.toBe(cause);
});

test("HTTP failures remain API errors rather than connection errors", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    Response.json({ error: "Permission denied" }, { status: 403 }),
  );
  await expect(
    readJSON(await apiFetch("/api/v1/monitoring")),
  ).rejects.toBeInstanceOf(APIError);
});

test("connection loss while reading the response body is also classified", async () => {
  const response = Response.json({});
  vi.spyOn(response, "json").mockRejectedValue(
    new TypeError("Body stream failed"),
  );
  await expect(readJSON(response)).rejects.toBeInstanceOf(ConnectionError);
});
