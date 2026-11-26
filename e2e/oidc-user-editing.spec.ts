import { test, expect, prepareScreenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("admin drawer submits an SSO account without a password", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/v1/auth/session", async (route) => {
    const response = await route.fetch();
    await route.fulfill({
      response,
      json: {
        ...(await response.json()),
        oidc_enabled: true,
        oidc_auto_link_email: true,
      },
    });
  });
  await page.reload();
  await openScreen(page, "users", "Users");
  await page
    .getByRole("button", { name: "+ Create user", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Create user" });
  await drawer.getByLabel("Username", { exact: true }).fill("future-user");
  await drawer
    .getByLabel("Email address", { exact: true })
    .fill("Élodie@example.test");
  await expect(
    drawer.getByText("Leave blank to use OpenID Connect sign-in only."),
  ).toBeVisible();
  await page.route("**/api/v1/users", async (route) => {
    if (route.request().method() !== "POST") return route.continue();
    const input = route.request().postDataJSON();
    await route.fulfill({
      status: 201,
      json: {
        ...input,
        id: 42,
        role: { id: input.role_id, name: "User", scope: "platform" },
        created_at: 1,
      },
    });
  });
  const submitted = page.waitForRequest(
    (req) => req.method() === "POST" && req.url().endsWith("/api/v1/users"),
  );
  await drawer.getByRole("button", { name: "Save user", exact: true }).click();
  expect((await submitted).postDataJSON()).toMatchObject({
    username: "future-user",
    email: "Élodie@example.test",
    password: "",
  });
  await expect(drawer).toBeHidden();
});

test("the admin drawer keeps OIDC names and emails read-only", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "users", "Users");
  await page
    .getByRole("button", { name: "Edit user Sam Chen", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Edit user", exact: true });
  const name = drawer.getByRole("textbox", {
    name: "Name (optional)",
    exact: true,
  });
  const email = drawer.getByRole("textbox", {
    name: "Email address",
    exact: true,
  });
  await expect(name).not.toBeEditable();
  await expect(email).not.toBeEditable();
  await prepareScreenshot(page);
  await expect(drawer).toHaveScreenshot(["crops", "user-oidc-edit.png"]);
  await drawer.getByRole("button", { name: "Save user", exact: true }).click();
  await expect(drawer).toBeHidden();
});

test("a linked user signing in locally sees provider-managed profile details", async ({
  page,
}) => {
  await login(page, "sam");
  await page
    .getByRole("button", { name: "Account options for Sam Chen", exact: true })
    .click();
  await page.getByRole("button", { name: "My profile", exact: true }).click();
  await expect(
    page.getByLabel("Name (optional)", { exact: true }),
  ).not.toBeEditable();
  await expect(
    page.getByLabel("Email address", { exact: true }),
  ).not.toBeEditable();
});
