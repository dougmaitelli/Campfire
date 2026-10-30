import { test, expect, screenshot } from "./fixtures";
import { login } from "./screens";

test("only Admin can start a persistent read-only user preview and return to their account", async ({
  page,
}) => {
  await login(page);
  await page
    .getByRole("button", { name: "Account options for Developer", exact: true })
    .click();
  await page.getByRole("button", { name: "View as…", exact: true }).click();
  const drawer = page.getByRole("dialog", { name: "View as…", exact: true });
  await expect(
    drawer.getByRole("button", { name: "Start preview", exact: true }),
  ).toBeDisabled();
  await drawer.getByRole("combobox", { name: "User", exact: true }).click();
  await expect(page.getByRole("option", { name: /Developer/ })).toHaveCount(0);
  await expect(page.getByRole("option", { name: /disabled/ })).toHaveCount(0);
  await page
    .getByRole("option", { name: "Schedule Observer (observer)", exact: true })
    .click();
  await screenshot(page, "view-as-drawer.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "view-as-drawer-mobile.png");
  await drawer
    .getByRole("button", { name: "Start preview", exact: true })
    .click();
  const preview = page.getByRole("region", {
    name: "Permission preview",
    exact: true,
  });
  await expect(preview).toContainText(
    "Viewing as Schedule Observer (observer)",
  );
  await expect(preview).toContainText("Read-only preview");
  await expect(
    page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("link", { name: "Users", exact: true }),
  ).toHaveCount(0);
  await expect(
    page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("link", { name: "Sparks", exact: true }),
  ).toHaveCount(0);
  const session = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as {
    csrf_token: string;
    user_id: number;
    admin: boolean;
    view_as: { admin_user_id: number };
  };
  expect(session.admin).toBe(false);
  expect(session.view_as.admin_user_id).not.toBe(session.user_id);
  const serversResponse = await page.request.get("/api/v1/servers");
  const servers = (await serversResponse.json()) as { id: string }[];
  expect(servers.map((server) => server.id)).toEqual(["survival"]);
  expect((await page.request.get("/api/v1/servers/creative")).status()).toBe(
    404,
  );
  expect((await page.request.get("/api/v1/users")).status()).toBe(403);
  expect((await page.request.get("/api/v1/tokens")).status()).toBe(403);
  const denied = await page.request.post("/api/v1/servers/survival/actions", {
    headers: { "X-CSRF-Token": session.csrf_token },
    data: { action: "stop" },
  });
  expect(denied.status()).toBe(403);
  expect(await denied.json()).toEqual({
    error: "View as is read-only. Return to Admin to make changes.",
  });
  await screenshot(page, "view-as-mobile.png");
  await page.setViewportSize({ width: 1400, height: 900 });
  await screenshot(page, "view-as.png");
  await page.reload();
  await expect(preview).toBeVisible();
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("link", { name: "Schedules", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Schedules", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "+ Create schedule", exact: true }),
  ).toBeDisabled();
  await preview
    .getByRole("button", { name: "Return to Admin", exact: true })
    .click();
  await expect(preview).toHaveCount(0);
  await expect(
    page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("link", { name: "Users", exact: true }),
  ).toBeVisible();
  const restored = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { admin: boolean; view_as?: unknown };
  expect(restored.admin).toBe(true);
  expect(restored.view_as).toBeUndefined();
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("link", { name: "Audit", exact: true })
    .click();
  await expect(
    page.getByText("auth.view_as.start", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("auth.view_as.stop", { exact: true }),
  ).toBeVisible();
  const events = (await (await page.request.get("/api/v1/audit")).json()) as {
    action: string;
    user_id: number;
    username: string;
  }[];
  const previewEvents = events.filter((event) =>
    event.action.startsWith("auth.view_as."),
  );
  expect(previewEvents).toHaveLength(3);
  for (const event of previewEvents) {
    expect(event.user_id).toBe(session.view_as.admin_user_id);
    expect(event.username).toBe("admin");
  }
});

test("ordinary users cannot see or invoke View as", async ({ page }) => {
  await login(page, "observer");
  await page
    .getByRole("button", {
      name: "Account options for Schedule Observer",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("button", { name: "View as…", exact: true }),
  ).toHaveCount(0);
  const session = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const response = await page.request.post("/api/v1/auth/view-as", {
    headers: { "X-CSRF-Token": session.csrf_token },
    data: { user_id: 1 },
  });
  expect(response.status()).toBe(403);
});

test("starting and stopping preview refreshes other tabs in the same session", async ({
  page,
  context,
}) => {
  await login(page);
  const sibling = await context.newPage();
  await sibling.goto("/");
  await expect(
    sibling.getByRole("navigation", { name: "Main navigation" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Account options for Developer", exact: true })
    .click();
  await page.getByRole("button", { name: "View as…", exact: true }).click();
  const drawer = page.getByRole("dialog", { name: "View as…", exact: true });
  await drawer.getByRole("combobox", { name: "User", exact: true }).click();
  await page
    .getByRole("option", { name: "Schedule Observer (observer)", exact: true })
    .click();
  await drawer
    .getByRole("button", { name: "Start preview", exact: true })
    .click();
  await expect(
    sibling.getByRole("region", { name: "Permission preview", exact: true }),
  ).toContainText("Schedule Observer");
  await expect(
    page.getByRole("region", { name: "Permission preview", exact: true }),
  ).toBeVisible();
  await sibling
    .getByRole("button", { name: "Return to Admin", exact: true })
    .click();
  await expect(
    page.getByRole("region", { name: "Permission preview", exact: true }),
  ).toHaveCount(0);
  await expect(
    page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("link", { name: "Users", exact: true }),
  ).toBeVisible();
  await sibling.close();
});
