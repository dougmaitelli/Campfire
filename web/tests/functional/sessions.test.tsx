import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { SessionsDrawer } from "../../src/drawers/SessionsDrawer";
import { AuthenticationProvider } from "../../src/hooks/useAuthentication";
import type { LoginSession } from "../../src/api/sessions";

const sessions: LoginSession[] = [
  {
    id: "current",
    user_agent: "Mozilla/5.0 (Macintosh) Version/18.0 Safari/605.1",
    current: true,
    created_at: 1700000000,
    last_seen_at: 1700000900,
    expires_at: 1800000000,
  },
  {
    id: "other",
    user_agent: "Mozilla/5.0 (Windows) Chrome/131.0 Safari/537.36",
    current: false,
    created_at: 1700000000,
    last_seen_at: 1700000800,
    expires_at: 1800000000,
  },
];

beforeEach(() => {
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute("open", "");
  };
  HTMLDialogElement.prototype.close = function () {
    this.removeAttribute("open");
  };
});
afterEach(() => vi.unstubAllGlobals());

function setup(fail = false, listed: LoginSession[] = sessions) {
  const close = vi.fn();
  const fetchMock = vi.fn(
    async (input: string | URL | Request, init?: RequestInit) => {
      const path = String(input);
      if (init?.method === "DELETE")
        return fail
          ? Response.json({ error: "Try again" }, { status: 400 })
          : new Response(null, { status: 204 });
      if (path.endsWith("/auth/sessions")) return Response.json(listed);
      return Response.json({
        authenticated: true,
        setup_required: false,
        csrf_token: "csrf",
      });
    },
  );
  vi.stubGlobal("fetch", fetchMock);
  render(
    <AuthenticationProvider>
      <SessionsDrawer onClose={close} />
    </AuthenticationProvider>,
  );
  return { fetchMock, close };
}

test("lists devices and cancels or revokes an individual session", async () => {
  const { fetchMock } = setup();
  await screen.findByText("This session");
  expect(screen.getByText("Safari · macOS")).toBeTruthy();
  fireEvent.click(
    screen.getByRole("button", { name: "Sign out Chrome · Windows" }),
  );
  let dialog = await screen.findByRole("dialog", { name: /^Sign out/ });
  await act(async () => {
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
  });
  expect(
    fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE"),
  ).toBe(false);
  fireEvent.click(
    screen.getByRole("button", { name: "Sign out Chrome · Windows" }),
  );
  dialog = await screen.findByRole("dialog", { name: /^Sign out/ });
  fireEvent.click(within(dialog).getByRole("button", { name: "Sign out" }));
  await waitFor(() =>
    expect(screen.queryByText("Chrome · Windows")).toBeNull(),
  );
  expect(screen.getByText("This session")).toBeTruthy();
  expect(
    fetchMock.mock.calls.some(
      ([path, init]) =>
        String(path).endsWith("/sessions/other") && init?.method === "DELETE",
    ),
  ).toBe(true);
});

test("bulk sign-out keeps this browser and errors preserve the list", async () => {
  setup(true);
  await screen.findByText("This session");
  fireEvent.click(
    screen.getByRole("button", { name: "Sign out all other devices" }),
  );
  fireEvent.click(
    within(await screen.findByRole("dialog", { name: /^Sign out/ })).getByRole(
      "button",
      {
        name: "Sign out",
      },
    ),
  );
  await screen.findByText(/Try again/);
  expect(screen.getByText("Chrome · Windows")).toBeTruthy();
  expect(screen.getByText("This session")).toBeTruthy();
});

test("bulk sign-out removes other sessions", async () => {
  const { fetchMock } = setup();
  await screen.findByText("This session");
  fireEvent.click(
    screen.getByRole("button", { name: "Sign out all other devices" }),
  );
  fireEvent.click(
    within(await screen.findByRole("dialog", { name: /^Sign out/ })).getByRole(
      "button",
      {
        name: "Sign out",
      },
    ),
  );
  await waitFor(() =>
    expect(screen.queryByText("Chrome · Windows")).toBeNull(),
  );
  expect(screen.getByText("This session")).toBeTruthy();
  expect(
    fetchMock.mock.calls.some(
      ([path, init]) =>
        String(path).endsWith("/auth/sessions") && init?.method === "DELETE",
    ),
  ).toBe(true);
});

test("revoking this session closes the drawer and reloads authentication", async () => {
  const { fetchMock, close } = setup();
  fireEvent.click(
    await screen.findByRole("button", { name: "Sign out This session" }),
  );
  const dialog = await screen.findByRole("dialog", { name: /^Sign out/ });
  expect(
    within(dialog).getByText("You will be signed out of this browser."),
  ).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "Sign out" }));
  await waitFor(() => expect(close).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(
      fetchMock.mock.calls.filter(([path]) =>
        String(path).endsWith("/auth/session"),
      ),
    ).toHaveLength(2),
  );
});

test.each([
  ["CriOS/131.0.6778.73", "Chrome"],
  ["FxiOS/132.0", "Firefox"],
  ["EdgiOS/131.0", "Edge"],
  ["Version/18.0", "Safari"],
])(
  "identifies iOS browser %s in the list and sign-out confirmation",
  async (token, browser) => {
    setup(false, [
      {
        ...sessions[1]!,
        user_agent: `Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) ${token} Mobile/15E148 Safari/604.1`,
      },
    ]);
    await screen.findByText(`${browser} · iPhone`);
    fireEvent.click(
      screen.getByRole("button", { name: `Sign out ${browser} · iPhone` }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "Sign out session",
    });
    expect(
      within(dialog).getByText(
        `Sign out ${browser} · iPhone? It will need to sign in again.`,
      ),
    ).toBeTruthy();
  },
);
