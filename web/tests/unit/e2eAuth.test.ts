import { expect, expectTypeOf, test, vi } from "vitest";
import { readAuthenticatedSession } from "../../../e2e/auth";
import type { AuthState } from "../../src/api/auth";

function response(body: unknown, status = 200) {
  return {
    ok: () => status >= 200 && status < 300,
    status: () => status,
    json: vi.fn(async () => body),
  };
}

test("E2E session setup preserves the canonical session and requires its token", async () => {
  const body: AuthState = {
    authenticated: true,
    setup_required: false,
    csrf_token: "test-csrf-token",
    user_id: 2,
    admin: false,
    view_as: { admin_user_id: 1, admin_username: "admin", admin_name: "Admin" },
  };
  const session = await readAuthenticatedSession(response(body));
  expect(session).toEqual(body);
  expectTypeOf(session.csrf_token).toEqualTypeOf<string>();
});

test.each([
  null,
  {},
  { authenticated: false, csrf_token: "test-csrf-token" },
  { authenticated: "true", csrf_token: "test-csrf-token" },
  { authenticated: true },
  { authenticated: true, csrf_token: "" },
  { authenticated: true, csrf_token: null },
  { authenticated: true, csrf_token: 1 },
])(
  "E2E session setup rejects missing authentication or token: %j",
  async (body) => {
    await expect(readAuthenticatedSession(response(body))).rejects.toThrow(
      "Expected an authenticated session with a CSRF token",
    );
  },
);

test("E2E session setup rejects failed HTTP responses before reading their body", async () => {
  const failed = response(
    { authenticated: true, csrf_token: "test-csrf-token" },
    401,
  );
  await expect(readAuthenticatedSession(failed)).rejects.toThrow("HTTP 401");
  expect(failed.json).not.toHaveBeenCalled();
});
