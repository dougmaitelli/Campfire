import { test, expect, screenshot } from "./fixtures";
import {
  login,
  screens,
  openScreen,
  openUser,
  inspectServer,
  serverTabs,
  openServerTab,
} from "./screens";

test("login validation, persistent session, and logout", async ({ page }) => {
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Sign in", exact: true }),
  ).toBeVisible();
  await screenshot(page, "login.png");
  await page.getByLabel("Password", { exact: true }).fill("incorrect-password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(page.getByRole("alert")).toBeVisible();
  await login(page);
  await page.reload();
  await expect(
    page.getByRole("navigation", { name: "Main navigation" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Account options for Developer", exact: true })
    .click();
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Sign in", exact: true }),
  ).toBeVisible();
});

for (const [id, heading] of screens) {
  test(`${heading} page`, async ({ page }) => {
    await login(page);
    await openScreen(page, id, heading);
    await screenshot(page, `${id}.png`);
  });
}

for (const [tab, label] of serverTabs) {
  test(`server ${label.toLowerCase()} screen`, async ({ page }) => {
    await login(page);
    await openServerTab(page, tab, label);
    await screenshot(page, `server-${tab}.png`);
  });
}

test("console commands arrive in the live log stream", async ({ page }) => {
  await login(page);
  await openServerTab(page, "console", "Console");
  await page
    .getByRole("textbox", { name: "Command", exact: true })
    .fill("list");
  await page.getByRole("button", { name: "Send command", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Console output", exact: true }),
  ).toContainText("Command accepted.");
});

test("server file edits persist after reopening", async ({ page }) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  await page
    .getByRole("textbox", { name: "server.properties", exact: true })
    .fill("motd=Updated community server\n");
  await page.getByRole("button", { name: "Save file", exact: true }).click();
  await page.reload();
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  await expect(
    page.getByRole("textbox", { name: "server.properties", exact: true }),
  ).toHaveValue("motd=Updated community server\n");
});

test("user edits persist through the real API", async ({ page }) => {
  await login(page);
  await openUser(page);
  await screenshot(page, "user-drawer.png");
  await page
    .getByRole("dialog")
    .getByLabel("Name (optional)", { exact: true })
    .fill("Alex Updated");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Save user", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.reload();
  await expect(
    page.getByRole("button", { name: "Edit user Alex Updated", exact: true }),
  ).toBeVisible();
});

test("resource inspection keeps the current page and supports nested drawers", async ({
  page,
}) => {
  await login(page);
  await inspectServer(page);
  await expect(page).toHaveURL(/#schedules$/);
  await screenshot(page, "server-inspection.png");
  await page
    .getByRole("dialog")
    .getByRole("link", { name: "Europe", exact: true })
    .click();
  await expect(page.locator("dialog[open]")).toHaveCount(2);
  await expect(page).toHaveURL(/#schedules$/);
  await page
    .locator("dialog[open]")
    .last()
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await expect(page.locator("dialog[open]")).toHaveCount(1);
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page).toHaveURL(/#schedules$/);
});

test("schedule history excludes direct server operations", async ({ page }) => {
  await login(page);
  await openScreen(page, "servers", "Servers");
  await page
    .getByRole("button", { name: "Start Creative", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Stop Creative", exact: true }),
  ).toBeEnabled();
  await openScreen(page, "schedules", "Schedules");
  await expect(page.getByRole("table").last().locator("tbody tr")).toHaveCount(
    1,
  );
  const response = await page.request.get("/api/v1/schedules/executions");
  expect(response.ok()).toBeTruthy();
  expect(await response.json()).toEqual([
    expect.objectContaining({
      id: "scheduled-run",
      schedule_id: "nightly-restart",
    }),
  ]);
  await page.getByRole("button", { name: "Run now", exact: true }).click();
  await expect(page.getByRole("table").last().locator("tbody tr")).toHaveCount(
    2,
  );
});

test("non-admin users cannot open administrator pages", async ({ page }) => {
  await login(page, "viewer");
  await expect(
    page
      .getByRole("navigation", { name: "Main navigation" })
      .getByRole("link", { name: "Users", exact: true }),
  ).toHaveCount(0);
  await page.goto("/#users");
  await expect(
    page.getByText("You do not have access to this page", { exact: true }),
  ).toBeVisible();
  expect((await page.request.get("/api/v1/users")).status()).toBe(403);
});
