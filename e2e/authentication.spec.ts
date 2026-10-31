import { test, expect, screenshot } from "./fixtures";

test("mobile login supports password visibility and keyboard submission", async ({
  page,
}) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Sign in", exact: true }),
  ).toBeVisible();
  await screenshot(page, "login-mobile.png");
  const password = page.getByLabel("Password", { exact: true });
  await password.fill("campfire-e2e-password");
  await page
    .getByRole("button", { name: "Show password", exact: true })
    .click();
  await expect(password).toHaveAttribute("type", "text");
  await expect(password).toHaveValue("campfire-e2e-password");
  await page
    .getByRole("button", { name: "Hide password", exact: true })
    .click();
  await expect(password).toHaveAttribute("type", "password");
  await page.setViewportSize({ width: 320, height: 568 });
  await password.press("Enter");
  await expect(
    page.getByRole("navigation", { name: "Main navigation" }),
  ).toBeVisible();
});

for (const localLogin of [false, true]) {
  test(`OpenID Connect login with local login ${localLogin ? "enabled" : "disabled"}`, async ({
    page,
  }) => {
    await page.route("**/api/v1/auth/session", (route) =>
      route.fulfill({
        json: {
          authenticated: false,
          setup_required: false,
          local_login: localLogin,
          oidc_enabled: true,
        },
      }),
    );
    await page.goto("/");
    const provider = page.getByRole("link", {
      name: "Sign in with OIDC",
    });
    await expect(provider).toHaveAttribute("href", "/api/v1/auth/oidc/start");
    await expect(page.getByLabel("Username", { exact: true })).toHaveCount(
      localLogin ? 1 : 0,
    );
    await expect(page.getByLabel("Password", { exact: true })).toHaveCount(
      localLogin ? 1 : 0,
    );
    await expect(page.getByText(/Forgot your password/)).toHaveCount(
      localLogin ? 1 : 0,
    );
    await expect(
      page.getByText("or use your username", { exact: true }),
    ).toHaveCount(localLogin ? 1 : 0);
    const state = localLogin ? "combined" : "oidc";
    await screenshot(page, `login-${state}.png`);
    await page.setViewportSize({ width: 375, height: 812 });
    await screenshot(page, `login-${state}-mobile.png`);
    await page.setViewportSize({ width: 320, height: 568 });
    await page.goto("/?oidc_error=failed");
    await expect(page.getByRole("alert")).toContainText(
      "OpenID Connect sign-in failed",
    );
    await expect(provider).toBeVisible();
    await page.route("**/api/v1/auth/oidc/start", (route) =>
      route.fulfill({
        contentType: "text/html",
        body: "<h1>Identity provider</h1>",
      }),
    );
    await provider.click();
    await expect(page).toHaveURL(/\/api\/v1\/auth\/oidc\/start$/);
    await expect(
      page.getByRole("heading", { name: "Identity provider" }),
    ).toBeVisible();
  });
}

test("administrator setup keeps all fields accessible on desktop and mobile", async ({
  page,
}) => {
  await page.route("**/api/v1/auth/session", (route) =>
    route.fulfill({
      json: {
        authenticated: false,
        setup_required: true,
        local_login: true,
        oidc_enabled: true,
      },
    }),
  );
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Set up your administrator" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Sign in with OIDC" }),
  ).toHaveCount(0);
  await screenshot(page, "login-setup.png");
  await page.setViewportSize({ width: 375, height: 812 });
  await screenshot(page, "login-setup-mobile.png");
  await page.getByLabel("Setup token").fill("setup-token");
  await page
    .getByLabel("Password", { exact: true })
    .fill("a long test passphrase");
  await page
    .getByLabel("Confirm password", { exact: true })
    .fill("a different passphrase");
  await page.getByRole("button", { name: "Create administrator" }).click();
  await expect(page.getByRole("alert")).toHaveText(
    "The passwords do not match.",
  );
  await page
    .getByLabel("Confirm password", { exact: true })
    .fill("a long test passphrase");
  await page.route("**/api/v1/auth/setup", async (route) => {
    expect(route.request().postDataJSON()).toEqual({
      username: "admin",
      password: "a long test passphrase",
      setup_token: "setup-token",
    });
    await route.fulfill({
      status: 400,
      json: { error: "The setup token is invalid." },
    });
  });
  await page.getByRole("button", { name: "Create administrator" }).click();
  await expect(page.getByRole("alert")).toHaveText(
    "The setup token is invalid.",
  );
});
