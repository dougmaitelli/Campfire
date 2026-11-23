import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { MFAForm } from "../../src/components/navigation/MFAForm";
import { MFADevices } from "../../src/components/navigation/MFADevices";
import { AdminMFAReset } from "../../src/components/navigation/AdminMFAReset";
import { AuthenticationPage } from "../../src/pages/AuthenticationPage";
import { AuthenticationProvider } from "../../src/hooks/useAuthentication";
import { MyProfileDrawer } from "../../src/drawers/MyProfileDrawer";

test.each(["enroll", "remove"])(
  "profile retries failed MFA refresh after %s without losing edits",
  async (operation) => {
    let changed = false;
    let failRefresh = true;
    const user = {
      id: 2,
      username: "friend",
      name: "Friend",
      email: "",
      role_id: 2,
      role: { id: 2, name: "User", scope: "platform" },
      disabled: false,
      created_at: 1700000000,
    };
    const fetchMock = vi.fn(async (path: string, init?: RequestInit) => {
      if (path.endsWith("/profile"))
        return Response.json({ user, local_password: true });
      if (path.endsWith("/enroll"))
        return Response.json({
          secret: "SECRETKEY",
          uri: "otpauth://totp/Campfire",
          qr_code: "data:image/png;base64,cG5n",
          expires_at: 2000000000,
        });
      if (path.endsWith("/confirm")) {
        changed = true;
        return Response.json({ recovery_codes: ["saved-code"] });
      }
      if (init?.method === "DELETE") {
        changed = true;
        return new Response(null, { status: 204 });
      }
      if (path.endsWith("/mfa")) {
        if (changed && failRefresh)
          return Response.json(
            { error: "Could not refresh authenticators" },
            { status: 503 },
          );
        const enabled = operation === "enroll" ? changed : !changed;
        return Response.json({
          enabled,
          available: true,
          recovery_remaining: enabled ? 10 : 0,
          devices: enabled
            ? [{ id: "phone", name: "Phone", created_at: 1700000000 }]
            : [],
        });
      }
      return Response.json({
        authenticated: true,
        setup_required: false,
        local_login: true,
        oidc_enabled: false,
        username: "friend",
        user_id: 2,
        role_id: 2,
        role: "User",
        admin: false,
        permissions: [],
        csrf_token: "csrf",
        expires_at: 2000000000,
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AuthenticationProvider>
        <MyProfileDrawer onClose={vi.fn()} />
      </AuthenticationProvider>,
    );
    fireEvent.change(await screen.findByLabelText("Name (optional)"), {
      target: { value: "Unsaved name" },
    });
    if (operation === "enroll") {
      fireEvent.click(
        screen.getByRole("button", { name: "Add authenticator" }),
      );
      fireEvent.change(await screen.findByLabelText("Device name"), {
        target: { value: "Phone" },
      });
      fireEvent.click(screen.getByRole("button", { name: "Show QR code" }));
      fireEvent.change(await screen.findByLabelText("Authenticator code"), {
        target: { value: "123456" },
      });
      fireEvent.click(
        screen.getByRole("button", { name: "Confirm authenticator" }),
      );
      fireEvent.click(
        await screen.findByRole("checkbox", {
          name: "I have saved my recovery codes",
        }),
      );
      fireEvent.click(screen.getByRole("button", { name: "Done" }));
    } else {
      fireEvent.click(screen.getByRole("button", { name: "Remove Phone" }));
      const dialog = screen.getByRole("dialog", { name: "Remove Phone" });
      fireEvent.change(
        within(dialog).getByLabelText("Authenticator or recovery code"),
        { target: { value: "123456" } },
      );
      fireEvent.click(within(dialog).getByRole("button", { name: "Remove" }));
    }
    await screen.findByText(/Could not refresh authenticators/);
    expect(
      screen.queryByRole("table", { name: "Authenticator devices" }),
    ).toBeNull();
    expect(
      (
        screen.getByRole("button", {
          name: "Save profile",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(
      (screen.getByLabelText("Name (optional)") as HTMLInputElement).value,
    ).toBe("Unsaved name");
    failRefresh = false;
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() =>
      expect(
        (
          screen.getByRole("button", {
            name: "Save profile",
          }) as HTMLButtonElement
        ).disabled,
      ).toBe(false),
    );
    expect(screen.queryByText(/Could not refresh authenticators/)).toBeNull();
    expect(
      (screen.getByLabelText("Name (optional)") as HTMLInputElement).value,
    ).toBe("Unsaved name");
    expect(!!screen.queryByRole("button", { name: "Remove Phone" })).toBe(
      operation === "enroll",
    );
    fireEvent.change(screen.getByLabelText("New password (optional)"), {
      target: { value: "New test password1!" },
    });
    fireEvent.change(screen.getByLabelText("Repeat password"), {
      target: { value: "New test password1!" },
    });
    fireEvent.change(screen.getByLabelText("Current password"), {
      target: { value: "old password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save profile" }));
    if (operation === "enroll") {
      await screen.findByRole("dialog", { name: "Verify password change" });
      expect(
        fetchMock.mock.calls.some(([, init]) => init?.method === "PUT"),
      ).toBe(false);
    } else {
      await waitFor(() =>
        expect(
          fetchMock.mock.calls.some(([, init]) => init?.method === "PUT"),
        ).toBe(true),
      );
      expect(
        screen.queryByRole("dialog", { name: "Verify password change" }),
      ).toBeNull();
    }
  },
);

beforeEach(() => {
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute("open", "");
  };
  HTMLDialogElement.prototype.close = function () {
    this.removeAttribute("open");
  };
});

test("enrollment displays a local QR and requires saving recovery codes", async () => {
  const completed = vi.fn();
  const fetchMock = vi.fn(async (path: string) => {
    if (path.endsWith("/enroll"))
      return Response.json({
        secret: "SECRETKEY",
        uri: "otpauth://totp/Campfire",
        qr_code: "data:image/png;base64,cG5n",
        expires_at: 2000000000,
      });
    if (path.endsWith("/confirm"))
      return Response.json({
        recovery_codes: ["11111111-22222222-33333333-44444444"],
      });
    return Response.json({
      enabled: false,
      available: true,
      recovery_remaining: 0,
      devices: [],
    });
  });
  vi.stubGlobal("fetch", fetchMock);
  render(<MFAForm onComplete={completed} onBusy={vi.fn()} onClose={vi.fn()} />);
  expect(screen.queryByLabelText("Password")).toBeNull();
  fireEvent.change(await screen.findByLabelText("Device name"), {
    target: { value: "Personal phone" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Show QR code" }));
  const qr = await screen.findByRole("img", {
    name: "Authenticator setup QR code",
  });
  expect(qr.getAttribute("src")).toMatch(/^data:image\/png;base64,/);
  expect(screen.getByText("SECRETKEY")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Authenticator code"), {
    target: { value: "123456" },
  });
  fireEvent.click(
    screen.getByRole("button", { name: "Confirm authenticator" }),
  );
  await screen.findByText("11111111-22222222-33333333-44444444");
  expect(
    (screen.getByRole("button", { name: "Done" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.queryByText("SECRETKEY")).toBeNull();
  fireEvent.click(
    screen.getByRole("checkbox", { name: "I have saved my recovery codes" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "Done" }));
  expect(completed).toHaveBeenCalledOnce();
});

test("login requests a second factor without losing password state", async () => {
  let loginAttempts = 0;
  const fetchMock = vi.fn(async (path: string, init?: RequestInit) => {
    if (path.endsWith("/login")) {
      loginAttempts++;
      if (loginAttempts === 1)
        return Response.json(
          { error: "code required", two_factor_required: true },
          { status: 401 },
        );
      expect(JSON.parse(String(init?.body))).toEqual({
        username: "admin",
        password: "private password",
        code: "654321",
      });
      return Response.json({
        authenticated: true,
        setup_required: false,
        local_login: true,
        oidc_enabled: false,
        username: "admin",
        user_id: 1,
        role_id: 1,
        role: "Admin",
        admin: true,
        permissions: [],
        csrf_token: "csrf",
        expires_at: 2000000000,
      });
    }
    return Response.json({
      authenticated: false,
      setup_required: false,
      local_login: true,
      oidc_enabled: false,
    });
  });
  vi.stubGlobal("fetch", fetchMock);
  render(
    <AuthenticationProvider>
      <AuthenticationPage />
    </AuthenticationProvider>,
  );
  fireEvent.change(await screen.findByLabelText("Password"), {
    target: { value: "private password" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  fireEvent.change(await screen.findByLabelText("Authenticator code"), {
    target: { value: "654321" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  await waitFor(() => expect(loginAttempts).toBe(2));
});

test("lost authenticator link signs in with a recovery code without resetting MFA", async () => {
  let attempts = 0;
  const fetchMock = vi.fn(async (path: string, init?: RequestInit) => {
    if (path.endsWith("/login")) {
      attempts++;
      if (attempts === 1)
        return Response.json({ two_factor_required: true }, { status: 401 });
      expect(JSON.parse(String(init?.body))).toEqual({
        username: "admin",
        password: "existing password",
        code: "saved-recovery-code",
      });
      return Response.json({ error: "Invalid recovery code" }, { status: 401 });
    }
    return Response.json({
      authenticated: false,
      setup_required: false,
      local_login: true,
      oidc_enabled: false,
    });
  });
  vi.stubGlobal("fetch", fetchMock);
  render(
    <AuthenticationProvider>
      <AuthenticationPage />
    </AuthenticationProvider>,
  );
  const signIn = await screen.findByRole("button", { name: "Sign in" });
  expect(
    screen.queryByRole("button", { name: "Lost your authenticator?" }),
  ).toBeNull();
  fireEvent.change(screen.getByLabelText("Password"), {
    target: { value: "existing password" },
  });
  fireEvent.click(signIn);
  fireEvent.click(
    await screen.findByRole("button", {
      name: "Lost your authenticator?",
    }),
  );
  expect(screen.queryByText(/reset-password command/)).toBeNull();
  fireEvent.change(screen.getByLabelText("Password"), {
    target: { value: "existing password" },
  });
  fireEvent.change(screen.getByLabelText("Recovery code"), {
    target: { value: "saved-recovery-code" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
  await screen.findByRole("alert");
  fireEvent.click(
    screen.getByRole("button", { name: "Use an authenticator code" }),
  );
  expect(screen.queryByLabelText("Recovery code")).toBeNull();
  expect(screen.getByLabelText("Authenticator code")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Sign in" })).toBeTruthy();
});

test("device table removes only the selected authenticator after confirmation", async () => {
  const changed = vi.fn();
  const fetchMock = vi.fn(async (path: string) => {
    expect(path).toContain("/auth/mfa/devices/");
    return new Response(null, { status: 204 });
  });
  vi.stubGlobal("fetch", fetchMock);
  render(
    <MFADevices
      status={{
        enabled: true,
        available: true,
        recovery_remaining: 10,
        devices: [
          { id: "phone", name: "Phone", created_at: 1700000000 },
          { id: "tablet", name: "Tablet", created_at: 1700000000 },
        ],
      }}
      disabled={false}
      onChanged={changed}
      onBusy={vi.fn()}
    />,
  );
  expect(
    screen.getByRole("table", { name: "Authenticator devices" }),
  ).toBeTruthy();
  expect(screen.queryByLabelText("Password")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Remove Tablet" }));
  const dialog = screen.getByRole("dialog", { name: "Remove Tablet" });
  fireEvent.change(
    within(dialog).getByLabelText("Authenticator or recovery code"),
    { target: { value: "123456" } },
  );
  fireEvent.click(within(dialog).getByRole("button", { name: "Remove" }));
  await waitFor(() => expect(changed).toHaveBeenCalledOnce());
  expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/auth/mfa/devices/tablet");
});

test("admin reset uses the authenticated session and explicit confirmation", async () => {
  const fetchMock = vi.fn(async (_path: string, init?: RequestInit) => {
    if (init?.method === "POST") return new Response(null, { status: 204 });
    return Response.json({
      enabled: true,
      available: true,
      recovery_remaining: 10,
      devices: [{ id: "phone", name: "Phone", created_at: 1700000000 }],
    });
  });
  vi.stubGlobal("fetch", fetchMock);
  render(<AdminMFAReset userID={42} username="friend" onBusy={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "Reset user’s 2FA" }));
  expect(screen.queryByLabelText("Your administrator password")).toBeNull();
  expect(
    screen.queryByLabelText("Your authenticator or recovery code"),
  ).toBeNull();
  const dialog = await screen.findByRole("dialog", {
    name: "Reset user’s 2FA",
  });
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(
    false,
  );
  fireEvent.click(
    within(dialog).getByRole("button", { name: "Reset user’s 2FA" }),
  );
  await screen.findByText(
    "2FA removed for friend. They can sign in with their password. API tokens are unchanged.",
  );
  expect(
    fetchMock.mock.calls.some(
      ([path, init]) =>
        path === "/api/v1/users/42/reset-2fa" && String(init?.body) === "{}",
    ),
  ).toBe(true);
});

test.each([
  [false, false],
  [true, false],
  [true, true],
  [false, true],
])(
  "My profile preserves edits and confirms password changes (change=%s, MFA=%s)",
  async (changePassword, mfaEnabled) => {
    const { AccountMenu } =
      await import("../../src/components/navigation/AccountMenu");
    const user = {
      id: 2,
      username: "friend",
      name: "Friend",
      email: "",
      role_id: 2,
      role: { id: 2, name: "User", scope: "platform" },
      disabled: false,
      created_at: 1700000000,
    };
    const fetchMock = vi.fn(async (path: string, init?: RequestInit) => {
      if (path.endsWith("/profile")) {
        if (init?.method === "PUT")
          return Response.json({ ...user, name: "Updated" });
        return Response.json({ user, local_password: true });
      }
      if (path.endsWith("/auth/sessions")) return Response.json([]);
      if (path.endsWith("/tokens"))
        return Response.json({ tokens: [], scopes: [], permissions: [] });
      if (path.endsWith("/mfa"))
        return Response.json({
          enabled: mfaEnabled,
          available: true,
          recovery_remaining: 0,
          devices: mfaEnabled
            ? [{ id: "phone", name: "Phone", created_at: 1700000000 }]
            : [],
        });
      return Response.json({
        authenticated: true,
        setup_required: false,
        local_login: true,
        oidc_enabled: false,
        username: "friend",
        name: "Friend",
        user_id: 2,
        role_id: 2,
        role: "User",
        admin: false,
        permissions: [],
        csrf_token: "csrf",
        expires_at: 2000000000,
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AuthenticationProvider>
        <AccountMenu />
      </AuthenticationProvider>,
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Account options for Friend" }),
    );
    expect(screen.queryByRole("button", { name: "API tokens" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Active sessions" }),
    ).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "My profile" }));
    const name = await screen.findByLabelText("Name (optional)");
    const newPassword = screen.getByLabelText("New password (optional)");
    expect(screen.queryByText(/Changing your password signs out/)).toBeNull();
    expect(screen.queryByLabelText("Repeat password")).toBeNull();
    expect(screen.queryByLabelText("Current password")).toBeNull();
    fireEvent.change(newPassword, {
      target: { value: "a new long password A1!" },
    });
    fireEvent.change(screen.getByLabelText("Repeat password"), {
      target: { value: "different password" },
    });
    fireEvent.change(screen.getByLabelText("Current password"), {
      target: { value: "old long password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save profile" }));
    await screen.findByText("The new passwords do not match.");
    expect(
      fetchMock.mock.calls.some(([, init]) => init?.method === "PUT"),
    ).toBe(false);
    fireEvent.change(newPassword, { target: { value: "" } });
    expect(screen.queryByLabelText("Repeat password")).toBeNull();
    expect(screen.queryByLabelText("Current password")).toBeNull();
    if (changePassword) {
      fireEvent.change(newPassword, {
        target: { value: "a new long password A1!" },
      });
      expect(
        (screen.getByLabelText("Repeat password") as HTMLInputElement).value,
      ).toBe("");
      expect(
        (screen.getByLabelText("Current password") as HTMLInputElement).value,
      ).toBe("");
      fireEvent.change(screen.getByLabelText("Repeat password"), {
        target: { value: "a new long password A1!" },
      });
      fireEvent.change(screen.getByLabelText("Current password"), {
        target: { value: "old long password" },
      });
    }
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.queryByRole("button", { name: "Delete" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Reset user’s 2FA" }),
    ).toBeNull();
    fireEvent.change(name, { target: { value: "Updated" } });
    // jsdom does not implement the browser's popover API.
    const actions = screen.getByRole("menu", { hidden: true });
    actions.removeAttribute("popover");
    Object.assign(actions, { hidePopover: vi.fn() });
    for (const title of ["API tokens", "Active sessions"]) {
      fireEvent.click(screen.getByRole("button", { name: "Actions" }));
      fireEvent.click(screen.getByRole("menuitem", { name: title }));
      const drawer = await screen.findByRole("dialog", { name: title });
      fireEvent.click(
        within(drawer).getByLabelText("Close", { selector: "button" }),
      );
      await waitFor(() =>
        expect(screen.queryByRole("dialog", { name: title })).toBeNull(),
      );
      expect(
        (screen.getByLabelText("Name (optional)") as HTMLInputElement).value,
      ).toBe("Updated");
    }
    fireEvent.click(screen.getByRole("button", { name: "Add authenticator" }));
    const enrollmentDialog = await screen.findByRole("dialog", {
      name: "Add authenticator",
    });
    expect(within(enrollmentDialog).queryByLabelText("Password")).toBeNull();
    fireEvent.click(
      within(enrollmentDialog).getByRole("button", { name: "Cancel" }),
    );
    fireEvent.change(name, { target: { value: "Updated" } });
    expect(
      screen.queryByRole("dialog", { name: "Verify password change" }),
    ).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Save profile" }));
    if (changePassword && mfaEnabled) {
      let dialog = await screen.findByRole("dialog", {
        name: "Verify password change",
      });
      expect(
        fetchMock.mock.calls.some(([, init]) => init?.method === "PUT"),
      ).toBe(false);
      fireEvent(
        dialog,
        new Event("cancel", { bubbles: true, cancelable: true }),
      );
      expect(
        screen.queryByRole("dialog", { name: "Verify password change" }),
      ).toBeNull();
      expect(
        (screen.getByLabelText("Name (optional)") as HTMLInputElement).value,
      ).toBe("Updated");
      expect(
        (screen.getByLabelText("New password (optional)") as HTMLInputElement)
          .value,
      ).toBe("a new long password A1!");
      fireEvent.click(screen.getByRole("button", { name: "Save profile" }));
      dialog = await screen.findByRole("dialog", {
        name: "Verify password change",
      });
      fireEvent.change(
        within(dialog).getByLabelText("Authenticator or recovery code"),
        { target: { value: "123456" } },
      );
      fireEvent.click(
        within(dialog).getByRole("button", { name: "Save profile" }),
      );
    }
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          ([path, init]) => path.endsWith("/profile") && init?.method === "PUT",
        ),
      ).toBe(true),
    );
    const saved = fetchMock.mock.calls.find(
      ([, init]) => init?.method === "PUT",
    );
    expect(JSON.parse(String(saved?.[1]?.body))).toEqual({
      name: "Updated",
      email: "",
      password: changePassword ? "a new long password A1!" : "",
      current_password: changePassword ? "old long password" : "",
      code: changePassword && mfaEnabled ? "123456" : "",
    });
  },
);
