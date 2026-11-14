import { test, expect, screenshot, prepareScreenshot } from "./fixtures";
import { login, openScreen, openServerTab } from "./screens";
import { ExecutionState } from "../web/src/contracts/domains";

test("Overview and Servers collapse into rows while simple lists stay tables", async ({
  page,
}) => {
  await login(page);
  await page.setViewportSize({ width: 1400, height: 900 });
  await prepareScreenshot(page);
  await expect(page.locator(".overview-metrics")).toHaveScreenshot([
    "crops",
    "overview-tiles.png",
  ]);
  await page.setViewportSize({ width: 1600, height: 1000 });
  await screenshot(page, "dashboard-wide.png");
  await page.setViewportSize({ width: 1240, height: 1000 });
  await screenshot(page, "dashboard-resources-compact.png");
  await page.setViewportSize({ width: 1000, height: 1000 });
  await screenshot(page, "overview-server-rows.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await prepareScreenshot(page);
  await expect(page.locator(".dashboard-server-table")).toHaveScreenshot([
    "crops",
    "overview-servers-mobile.png",
  ]);
  await page.setViewportSize({ width: 320, height: 800 });
  await prepareScreenshot(page);
  await expect(page.locator(".overview-metrics")).toHaveScreenshot([
    "crops",
    "overview-tiles-narrow.png",
  ]);
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("button", { name: "Open navigation", exact: true })
    .click();
  const navigation = page.getByRole("dialog", {
    name: "Main navigation",
    exact: true,
  });
  await screenshot(page, "sidebar-mobile.png", navigation);
  await navigation.getByRole("link", { name: "Servers", exact: true }).click();
  await expect(navigation).toBeHidden();
  await expect(
    page.getByRole("heading", { name: "Servers", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 1366, height: 768 });
  await screenshot(page, "servers-laptop.png");
  await page.setViewportSize({ width: 1100, height: 900 });
  await screenshot(page, "servers-compact.png");
  await page.setViewportSize({ width: 1000, height: 1000 });
  await screenshot(page, "servers-rows.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await prepareScreenshot(page);
  await expect(page.locator(".servers-table")).toHaveScreenshot([
    "crops",
    "servers-table-mobile.png",
  ]);
  await openScreen(page, "sparks", "Sparks");
  await page.setViewportSize({ width: 1000, height: 1000 });
  await screenshot(page, "sparks-compact.png");
  await page.setViewportSize({ width: 940, height: 900 });
  await page
    .getByRole("button", { name: "View details for Europe", exact: true })
    .click();
  const spark = page.getByRole("dialog", { name: /^Europe/ });
  await expect(spark).toBeVisible();
  await spark
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await page.route("**/api/v1/schedules/executions?*", async (route) => {
    const response = await route.fetch();
    const runs = (await response.json()) as Record<string, unknown>[];
    await route.fulfill({
      response,
      json: runs.map((run) => ({
        ...run,
        state: ExecutionState.Failed,
        message: "Server restart failed.",
      })),
    });
  });
  await openScreen(page, "schedules", "Schedules");
  await page.setViewportSize({ width: 1000, height: 1000 });
  await expect(
    page.getByRole("button", { name: "Retry", exact: true }),
  ).toBeVisible();
  await screenshot(page, "schedules-compact.png");
});

test("simple user and role tables remain usable on mobile", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "users", "Users");
  await page.setViewportSize({ width: 320, height: 800 });
  await page
    .getByRole("button", { name: "Edit user Developer", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeHidden();

  await openScreen(page, "roles", "Roles");
  await page.setViewportSize({ width: 320, height: 800 });
  await page.getByRole("button", { name: "Admin", exact: true }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
});

test("mobile navigation supports dismissal, account controls and desktop resizing", async ({
  page,
}) => {
  await login(page);
  await page.setViewportSize({ width: 390, height: 844 });
  const toggle = page.getByRole("button", {
    name: "Open navigation",
    exact: true,
  });
  const navigation = page.getByRole("dialog", {
    name: "Main navigation",
    exact: true,
  });
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-expanded", "true");
  await expect(
    navigation.getByRole("link", { name: "Overview", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(
    navigation.getByRole("button", { name: "Close navigation", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(navigation).toBeHidden();
  await expect(toggle).toBeFocused();
  await toggle.click();
  await page.mouse.click(380, 100);
  await expect(navigation).toBeHidden();
  await toggle.click();
  await navigation
    .getByRole("button", { name: "Account options for Developer", exact: true })
    .click();
  await page.keyboard.press("Escape");
  await expect(navigation).toBeVisible();
  await expect(
    navigation.getByRole("button", { name: "API tokens", exact: true }),
  ).toBeHidden();
  await navigation
    .getByRole("button", { name: "Account options for Developer", exact: true })
    .click();
  await navigation
    .getByRole("button", { name: "API tokens", exact: true })
    .click();
  const tokens = page.getByRole("dialog", { name: "API tokens", exact: true });
  await expect(tokens).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(tokens).toBeHidden();
  await expect(navigation).toBeVisible();
  await navigation
    .getByRole("button", { name: "Account options for Developer", exact: true })
    .click();
  await navigation
    .getByRole("button", { name: "API tokens", exact: true })
    .click();
  await page.setViewportSize({ width: 1400, height: 900 });
  await expect(navigation).toBeHidden();
  await expect(tokens).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(tokens).toBeHidden();
  await expect(
    page.getByRole("navigation", { name: "Main navigation", exact: true }),
  ).toBeVisible();
});

test("mobile file rows keep selection, downloads and file details available", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("checkbox", { name: "Select all items", exact: true })
    .check();
  await expect(
    page.getByRole("checkbox", {
      name: "Select server.properties",
      exact: true,
    }),
  ).toBeChecked();
  await page
    .getByRole("checkbox", { name: "Select all items", exact: true })
    .uncheck();
  const download = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Download server.properties", exact: true })
    .click();
  expect((await download).suggestedFilename()).toBe("server.properties");
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  await expect(
    page.getByRole("textbox", { name: /server\.properties$/ }),
  ).toBeVisible();
});

test("an empty console stays clear of unnecessary scrolling", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/v1/servers/survival/console/stream", (route) =>
    route.fulfill({
      contentType: "text/event-stream",
      body: "event: ready\ndata: {}\n\nevent: ended\ndata: {}\n\n",
    }),
  );
  await page.goto("/#servers/survival/console");
  await expect(
    page.getByText("Waiting for server output…", { exact: true }),
  ).toBeVisible();
  for (const viewport of [
    { width: 1400, height: 1100 },
    { width: 390, height: 844 },
  ]) {
    await page.setViewportSize(viewport);
    await expect
      .poll(() =>
        page
          .locator(".xterm-viewport")
          .evaluate(
            (element) =>
              element.scrollHeight <= element.clientHeight &&
              element.scrollWidth <= element.clientWidth,
          ),
      )
      .toBe(true);
  }
});

test("long console output remains readable after resizing to mobile", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "console", "Console");
  await page.setViewportSize({ width: 390, height: 844 });
  await page
    .getByRole("textbox", { name: "Command", exact: true })
    .fill(`say ${"console-output-".repeat(20)}END`);
  await page.getByRole("button", { name: "Send command", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Command", exact: true }),
  ).toHaveValue("");
  await expect(
    page.getByRole("region", { name: "Console output", exact: true }),
  ).toContainText("END");
  await expect(page.locator(".xterm-accessibility-tree")).toContainText("END");
  await prepareScreenshot(page);
  await expect(page.locator(".server-terminal-window")).toHaveScreenshot([
    "crops",
    "console-long-output-mobile.png",
  ]);
  await page.setViewportSize({ width: 1400, height: 1100 });
  await expect(page.locator(".xterm-accessibility-tree")).toContainText("END");
  await prepareScreenshot(page);
  await expect(page.locator(".server-terminal-window")).toHaveScreenshot([
    "crops",
    "console-long-output.png",
  ]);
});
