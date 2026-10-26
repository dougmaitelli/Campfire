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

for (const [id, heading] of [
  ["servers", "Servers"],
  ["dashboard", "Overview"],
] as const) {
  test(`${heading} server rows can restart a running server`, async ({
    page,
  }) => {
    await login(page);
    await openScreen(page, id, heading);
    const restart = page.getByRole("button", {
      name: "Restart Survival",
      exact: true,
    });
    await expect(restart).toBeEnabled();
    await expect(
      page.getByRole("button", { name: "Restart Creative", exact: true }),
    ).toBeDisabled();
    const response = page.waitForResponse(
      (response) =>
        response.url().endsWith("/api/v1/servers/survival/actions") &&
        response.request().method() === "POST" &&
        response.request().postDataJSON().action === "restart",
    );
    await restart.click();
    expect((await response).ok()).toBeTruthy();
    await expect(page).toHaveURL(new RegExp(`#${id}$`));
  });
}

for (const [tab, label] of serverTabs) {
  test(`server ${label.toLowerCase()} screen`, async ({ page }) => {
    await login(page);
    await openServerTab(page, tab, label);
    await screenshot(page, `server-${tab}.png`);
  });
}

test("Spark resource layout is identical on direct entry and after visiting Servers", async ({
  page,
}) => {
  await login(page);
  await page.goto("/#sparks");
  const readings = page.locator(".sparks-table .resource-readings").first();
  await expect(readings.locator(".usage-reading")).toHaveCount(3);
  await expect(readings).toHaveCSS("display", "grid");
  const positions = () =>
    readings
      .locator(".usage-reading > .usage-value")
      .evaluateAll((elements) =>
        elements.map((element) =>
          Math.round(element.getBoundingClientRect().left),
        ),
      );
  const direct = await positions();
  expect(new Set(direct).size).toBe(1);
  await openScreen(page, "servers", "Servers");
  await openScreen(page, "sparks", "Sparks");
  await expect(readings.locator(".usage-reading")).toHaveCount(3);
  expect(await positions()).toEqual(direct);
  await page.setViewportSize({ width: 800, height: 900 });
  expect(new Set(await positions()).size).toBe(1);
  await expect(
    page.getByRole("heading", { level: 1, name: "Sparks", exact: true }),
  ).toBeVisible();
});

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

test("console search preserves toolbar height and supports keyboard navigation", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "console", "Console");
  const command = page.getByRole("textbox", { name: "Command", exact: true });
  await command.fill("list");
  await page.getByRole("button", { name: "Send command", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Console output", exact: true }),
  ).toContainText("Command accepted.");
  const toolbar = page.locator(".server-terminal-toolbar");
  const before = await toolbar.boundingBox();
  expect(before).not.toBeNull();
  await command.focus();
  await page.keyboard.press("Control+f");
  const search = page.getByRole("textbox", {
    name: "Search console",
    exact: true,
  });
  await expect(search).toBeFocused();
  const after = await toolbar.boundingBox();
  expect(after?.height).toBe(before?.height);
  expect(after?.y).toBe(before?.y);
  const bounds = await search.boundingBox();
  expect(bounds?.width).toBeLessThanOrEqual(640);
  const buttons = page.getByRole("search").getByRole("button");
  const positions = () =>
    buttons.evaluateAll((elements) =>
      elements.map((element) => {
        const { x, y, width, height } = element.getBoundingClientRect();
        return { x, y, width, height };
      }),
    );
  const initialPositions = await positions();
  await search.fill("no-such-console-output");
  await expect(page.getByText("No matches", { exact: true })).toBeVisible();
  expect(await search.boundingBox()).toEqual(bounds);
  expect(await positions()).toEqual(initialPositions);
  await search.fill("Command accepted.");
  await expect(page.getByText("No matches", { exact: true })).toHaveCount(0);
  await search.press("Enter");
  await search.press("Shift+Enter");
  await search.press("Escape");
  await expect(page.getByRole("search")).toHaveCount(0);
  await expect(command).toBeVisible();
});

test("server file edits persist after reopening", async ({ page }) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  await page
    .getByRole("textbox", { name: "/server.properties", exact: true })
    .fill("motd=Updated community server\n");
  await page.getByRole("button", { name: "Save file", exact: true }).click();
  await page.reload();
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  await expect(
    page.getByRole("textbox", { name: "/server.properties", exact: true }),
  ).toHaveValue("motd=Updated community server\n");
});

test("file paths use the server data root and move defaults follow navigation", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  const move = page.getByRole("button", { name: "Move", exact: true });
  const dialog = page.getByRole("dialog", {
    name: "Move selected items",
    exact: true,
  });
  const destination = dialog.getByRole("textbox", {
    name: "Destination folder",
    exact: true,
  });
  await page
    .getByRole("checkbox", { name: "Select server.properties", exact: true })
    .check();
  await move.click();
  await expect(destination).toHaveValue("/");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "config", exact: true }).click();
  await page
    .getByRole("checkbox", { name: "Select permissions.json", exact: true })
    .check();
  await move.click();
  await expect(destination).toHaveValue("/config/");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "fabric", exact: true }).click();
  await page
    .getByRole("checkbox", { name: "Select settings.json", exact: true })
    .check();
  await move.click();
  await expect(destination).toHaveValue("/config/fabric/");
  await screenshot(page, "file-move-dialog.png");
  await dialog.getByRole("button", { name: "Move", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText("different");
  await destination.fill("/mods/");
  await dialog.getByRole("button", { name: "Move", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await page.getByRole("button", { name: "Server data", exact: true }).click();
  await page.getByRole("button", { name: "mods", exact: true }).click();
  await page
    .getByRole("button", { name: "settings.json", exact: true })
    .click();
  await expect(
    page.getByRole("textbox", { name: "/mods/settings.json", exact: true }),
  ).toHaveValue(/maxViewDistance/);
});

test("folder moves reject descendants and preserve nested file paths", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page
    .getByRole("checkbox", { name: "Select config", exact: true })
    .check();
  await page.getByRole("button", { name: "Move", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Move selected items",
    exact: true,
  });
  const destination = dialog.getByRole("textbox", {
    name: "Destination folder",
    exact: true,
  });
  await destination.fill("/config/fabric/");
  await dialog.getByRole("button", { name: "Move", exact: true }).click();
  await expect(dialog.getByRole("alert")).toBeVisible();
  await destination.fill("/world/");
  await dialog.getByRole("button", { name: "Move", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  for (const name of ["world", "config", "fabric", "settings.json"])
    await page.getByRole("button", { name, exact: true }).click();
  await expect(
    page.getByRole("textbox", {
      name: "/world/config/fabric/settings.json",
      exact: true,
    }),
  ).toBeVisible();
});

test("file permissions can be changed and persist after reloading", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  const permissions = page.getByRole("button", {
    name: "Permissions",
    exact: true,
  });
  await expect(permissions).toBeDisabled();
  await expect(permissions.locator("svg")).toHaveCount(1);
  for (const name of ["server.properties", "eula.txt", "config"]) {
    await page
      .getByRole("checkbox", { name: `Select ${name}`, exact: true })
      .check();
  }
  await page.getByRole("button", { name: "Permissions", exact: true }).click();
  await expect(
    page.getByLabel("Permissions (octal)", { exact: true }),
  ).toHaveValue("");
  await page.getByLabel("Permissions (octal)", { exact: true }).fill("755");
  await page
    .getByRole("button", { name: "Save permissions", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await page.reload();
  for (const name of ["server.properties", "eula.txt", "config"]) {
    const selected = page.getByRole("checkbox", {
      name: `Select ${name}`,
      exact: true,
    });
    await selected.check();
    await permissions.click();
    await expect(
      page.getByLabel("Permissions (octal)", { exact: true }),
    ).toHaveValue("755");
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await selected.uncheck();
  }
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
