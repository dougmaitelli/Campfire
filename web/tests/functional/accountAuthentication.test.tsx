import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { AccountMenu } from "../../src/components/navigation/AccountMenu";
import {
  AuthenticationProvider,
  useAuthentication,
} from "../../src/hooks/useAuthentication";
import type { AuthState } from "../../src/api/auth";

const session: AuthState = {
  authenticated: true,
  setup_required: false,
  local_login: false,
  oidc_enabled: true,
  oidc_linked: true,
  username: "oidc-user",
  user_id: 1,
  role_id: 2,
  role: "User",
  admin: false,
  permissions: [],
  csrf_token: "oidc-csrf",
  expires_at: 2000000000,
};

function Account() {
  const auth = useAuthentication();
  return auth.session?.authenticated ? <AccountMenu /> : <p>Signed out</p>;
}

afterEach(() => vi.unstubAllGlobals());

test.each([true, false])(
  "only unlinked accounts see Link OIDC (linked: %s)",
  async (linked) => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockImplementation(async () =>
          Response.json({ ...session, oidc_linked: linked }),
        ),
    );
    render(
      <AuthenticationProvider>
        <Account />
      </AuthenticationProvider>,
    );
    fireEvent.click(
      await screen.findByRole("button", {
        name: "Account options for oidc-user",
      }),
    );
    expect(
      screen.queryByRole("button", { name: /Link OIDC/i }) !== null,
    ).toBe(!linked);
  },
);

test("OIDC logout sends the session CSRF token and leaves the app signed out", async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(Response.json(session))
    .mockResolvedValueOnce(new Response(null, { status: 204 }))
    .mockResolvedValueOnce(
      Response.json({
        authenticated: false,
        setup_required: false,
        local_login: false,
        oidc_enabled: true,
        oidc_linked: false,
      }),
    );
  vi.stubGlobal("fetch", fetchMock);
  render(
    <AuthenticationProvider>
      <Account />
    </AuthenticationProvider>,
  );
  fireEvent.click(await screen.findByRole("button", { name: "Sign out" }));
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
  expect(fetchMock.mock.calls[1]![0]).toBe("/api/v1/auth/logout");
  const options = fetchMock.mock.calls[1]![1] as RequestInit;
  expect(options.method).toBe("POST");
  expect(new Headers(options.headers).get("X-CSRF-Token")).toBe("oidc-csrf");
  expect(await screen.findByText("Signed out")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Sign out" })).toBeNull();
});
