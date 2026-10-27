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
  await page.keyboard.press("Tab");
  await screenshot(page, "login-focused.png");
  await page.setViewportSize({ width: 375, height: 812 });
  await screenshot(page, "login-focused-mobile.png");
  await page.setViewportSize({ width: 1400, height: 900 });
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
    if (id === "dashboard") {
      await page.setViewportSize({ width: 390, height: 844 });
      await screenshot(page, "dashboard-mobile.png");
    }
  });
}

for (const screen of ["overview", "server"] as const) {
  test(`${screen} start action shows progress until the server is ready`, async ({
    page,
  }) => {
    await login(page);
    if (screen === "server") await page.goto("/#servers/creative/console");
    let state = "stopped";
    const status = () => ({
      server_id: "creative",
      state,
      ready: state === "running",
      busy: state === "starting",
      disk_quota_enforced: true,
      operation_id: "test-start",
      operation: "start",
      operation_state: state === "starting" ? "running" : "succeeded",
    });
    await page.route("**/api/v1/servers/creative/status", (route) =>
      route.fulfill({ json: status() }),
    );
    await page.route("**/api/v1/servers/creative/actions", (route) => {
      expect(route.request().postDataJSON().action).toBe("start");
      state = "starting";
      return route.fulfill({ json: status() });
    });
    const start = page.getByRole("button", {
      name: screen === "overview" ? "Start Creative" : "Start",
      exact: true,
    });
    const stateLabel =
      screen === "overview"
        ? page
            .getByRole("row")
            .filter({ hasText: "Creative" })
            .locator(".server-state")
        : page.locator(".page-header .server-state");
    await expect(start).toBeEnabled();
    await start.click();
    await expect(start).toBeDisabled();
    await expect(stateLabel).toContainText("Starting");
    if (screen === "server")
      await expect(page).toHaveScreenshot("server-starting.png");
    state = "running";
    await expect(stateLabel).toContainText("Running");
    await expect(
      page.getByRole("button", {
        name: screen === "overview" ? "Stop Creative" : "Stop",
        exact: true,
      }),
    ).toBeEnabled();
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

test("console shows CPU and memory history and keeps graphs out of other tabs", async ({
  page,
}) => {
  const to = Math.floor(new Date("2026-10-22T12:00:00Z").getTime() / 1000);
  const history = {
    from: to - 3600,
    to,
    interval_seconds: 300,
    samples: Array.from({ length: 13 }, (_, index) => ({
      collected_at: to - 3600 + index * 300,
      usage: {
        cpu_percent:
          index === 5
            ? null
            : [30, 55, 45, 90, 120, 0, 80, 100, 70, 110, 130, 100, 125][index],
        memory_bytes: index === 5 ? null : (768 + index * 64) * 1048576,
      },
    })),
  };
  await page.route("**/api/v1/servers/survival/metrics/history?*", (route) =>
    route.fulfill({ json: history }),
  );
  await login(page);
  await openServerTab(page, "console", "Console");
  const cpu = page.getByRole("region", { name: "CPU", exact: true });
  const memory = page.getByRole("region", { name: "Memory", exact: true });
  await expect(
    cpu.getByRole("img", { name: "CPU usage history" }),
  ).toBeVisible();
  await expect(cpu).toContainText("62.5%");
  await expect(
    memory.getByRole("img", { name: "Memory usage history" }),
  ).toBeVisible();
  await expect(memory).toContainText("1.5 GiB");
  await expect(page.locator(".resource-graphs")).toHaveScreenshot(
    "server-resource-graphs.png",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(page.locator(".resource-graphs")).toHaveScreenshot(
    "server-resource-graphs-mobile.png",
  );
  await page.getByRole("link", { name: "Files", exact: true }).click();
  await expect(cpu).toHaveCount(0);
  await page.getByRole("link", { name: "Console", exact: true }).click();
  await expect(
    cpu.getByRole("img", { name: "CPU usage history" }),
  ).toBeVisible();
});

test("console history handles missing samples and failed requests", async ({
  page,
}) => {
  const to = Math.floor(new Date("2026-10-22T12:00:00Z").getTime() / 1000);
  await page.route("**/api/v1/servers/survival/metrics/history?*", (route) =>
    route.fulfill({
      json: {
        from: to - 3600,
        to,
        interval_seconds: 300,
        samples: [
          {
            collected_at: to,
            usage: { cpu_percent: null, memory_bytes: null },
          },
        ],
      },
    }),
  );
  await login(page);
  await openServerTab(page, "console", "Console");
  await expect(
    page.getByText(
      "No samples yet. History will appear as measurements arrive.",
    ),
  ).toHaveCount(2);
  await page.route("**/api/v1/servers/survival/metrics/history?*", (route) =>
    route.fulfill({
      status: 408,
      json: { error: "History request timed out." },
    }),
  );
  await page.reload();
  await expect(
    page.getByRole("status").filter({ hasText: "History request timed out." }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Command", exact: true }),
  ).toBeVisible();
});

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
  await screenshot(page, "server-console-focused.png");
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

test("binary file rows remain selectable and downloadable without opening the editor", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  const row = page.getByRole("row").filter({ hasText: "server.jar" });
  await expect(
    row.getByRole("cell", { name: "JAR", exact: true }),
  ).toBeVisible();
  await expect(
    row.getByRole("button", { name: "server.jar", exact: true }),
  ).toBeDisabled();
  await row.getByRole("cell", { name: "JAR", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "/server.jar", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByRole("alert")).toHaveCount(0);
  const binaryDownload = row.getByRole("button", {
    name: "Download server.jar",
    exact: true,
  });
  const textDownload = page.getByRole("button", {
    name: "Download server.properties",
    exact: true,
  });
  await expect(async () => {
    const binaryBounds = await binaryDownload.boundingBox();
    const textBounds = await textDownload.boundingBox();
    expect(binaryBounds).not.toBeNull();
    expect(textBounds).not.toBeNull();
    expect(binaryBounds!.x).toBe(textBounds!.x);
  }).toPass();
  await row.getByRole("checkbox").check();
  const download = page.waitForEvent("download");
  await row
    .getByRole("button", { name: "Download server.jar", exact: true })
    .click();
  expect((await download).suggestedFilename()).toBe("server.jar");
  await screenshot(page, "file-binary-row.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(row.getByRole("checkbox")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "server.properties", exact: true }),
  ).toBeEnabled();
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
  const emptyFolder = page.getByText("This folder is empty.", { exact: true });
  await expect(emptyFolder).toBeVisible();
  await screenshot(page, "file-empty-folder.png");
  const emptyState = page
    .getByRole("status")
    .filter({ hasText: "This folder is empty." });
  const beforeHover = await emptyState.screenshot({
    animations: "disabled",
    caret: "hide",
  });
  await emptyFolder.hover();
  expect(
    await emptyState.screenshot({ animations: "disabled", caret: "hide" }),
  ).toEqual(beforeHover);
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
  const variable = page
    .getByRole("dialog")
    .locator(".details-list > div")
    .filter({ has: page.getByText("ENABLE_COMMAND_BLOCKS", { exact: true }) });
  await expect(variable).toHaveScreenshot("server-inspection-long-label.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(variable).toHaveScreenshot(
    "server-inspection-long-label-mobile.png",
  );
  await page.setViewportSize({ width: 1400, height: 900 });
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

  for (const [screen, heading, resource] of [
    ["servers", "Servers", "Survival"],
    ["sparks", "Sparks", "Europe"],
  ] as const) {
    await openScreen(page, screen, heading);
    await page
      .getByRole("button", {
        name: `View details for ${resource}`,
        exact: true,
      })
      .click();
    const dialog = page.getByRole("dialog");
    await expect(
      dialog.getByRole("heading", { level: 2, name: new RegExp(resource) }),
    ).toBeVisible();
    await expect(page.locator('[aria-busy="true"]')).toHaveCount(0);
    await expect(dialog.locator(".drawer-heading")).toHaveScreenshot(
      `${screen}-drawer-heading.png`,
    );
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(dialog.locator(".drawer-heading")).toHaveScreenshot(
      `${screen}-drawer-heading-mobile.png`,
    );
    await dialog
      .getByRole("button", { name: "Close", exact: true })
      .first()
      .click();
    await expect(dialog).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`#${screen}$`));
  }
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
