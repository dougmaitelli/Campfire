import { test, expect, prepareScreenshot, screenshot } from "./fixtures";
import { login, openScreen, openServerTab } from "./screens";
import { ExecutionState } from "../web/src/contracts/domains";

test("revoking an API token requires confirmation without closing its drawer", async ({
  page,
}) => {
  await login(page);
  await page
    .getByRole("button", { name: "Account options for Developer", exact: true })
    .click();
  await page.getByRole("button", { name: "API tokens", exact: true }).click();
  const drawer = page.getByRole("dialog", { name: "API tokens", exact: true });
  await drawer
    .getByRole("textbox", { name: "Name", exact: true })
    .fill("Dialog test token");
  await drawer
    .getByRole("button", { name: "Create token", exact: true })
    .click();
  const row = drawer
    .getByRole("heading", { name: "Dialog test token", exact: true })
    .locator("..");
  const revoke = row.getByRole("button", { name: "Revoke", exact: true });
  const dialog = page.getByRole("dialog", {
    name: "Revoke API token?",
    exact: true,
  });
  await revoke.click();
  await expect(dialog).toContainText("Dialog test token");
  await page.keyboard.press("Escape");
  await expect(drawer).toBeVisible();
  await expect(row).toBeVisible();
  await revoke.click();
  await dialog.getByRole("button", { name: "Revoke", exact: true }).click();
  await expect(row).toHaveCount(0);
  await expect(drawer).toBeVisible();
});

test("retrying schedule history requires confirmation before submitting", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/v1/schedules/executions?*", async (route) => {
    const response = await route.fetch();
    const items = (await response.json()) as { state: ExecutionState }[];
    await route.fulfill({
      response,
      json: items.map((item) => ({ ...item, state: ExecutionState.Failed })),
    });
  });
  const requests: string[] = [];
  await page.route("**/api/v1/schedules/executions/*/retry", async (route) => {
    requests.push(route.request().url());
    await route.fulfill({
      status: 400,
      json: { error: "Retry was rejected." },
    });
  });
  await openScreen(page, "schedules", "Schedules");
  const retry = page.getByRole("button", { name: "Retry", exact: true });
  const dialog = page.getByRole("dialog", {
    name: "Retry operation?",
    exact: true,
  });
  await retry.click();
  await expect(dialog).toContainText(
    "an interrupted operation may already have executed",
  );
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(requests).toEqual([]);
  await retry.click();
  await dialog.getByRole("button", { name: "Retry", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("Retry was rejected.");
  expect(requests).toHaveLength(1);
});

test("backup deletion confirms the copy and cancels when leaving the page", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "backups", "Backups");
  const requests: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "DELETE" && request.url().includes("/backups/"))
      requests.push(request.url());
  });
  const row = page.getByRole("row").filter({ hasText: "mock-nightly.tar.gz" });
  const remove = row.getByRole("button", { name: "Delete local", exact: true });
  const dialog = page.getByRole("dialog", {
    name: "Delete backup?",
    exact: true,
  });
  await remove.click();
  await expect(dialog).toContainText("local copy of mock-nightly.tar.gz");
  await expect(dialog).toContainText("Other copies are kept");
  await screenshot(page, "confirmation-delete-backup.png", dialog);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(requests).toEqual([]);
  await expect(row).toBeVisible();
  await remove.click();
  await expect(dialog).toBeVisible();
  await page.goto("/#servers");
  await expect(dialog).toHaveCount(0);
  expect(requests).toEqual([]);
  await openServerTab(page, "backups", "Backups");
  await remove.click();
  await page.setViewportSize({ width: 320, height: 844 });
  const confirm = dialog.getByRole("button", {
    name: "Delete local",
    exact: true,
  });
  const response = page.waitForResponse(
    (response) =>
      response.request().method() === "DELETE" &&
      response.url().includes("/backups/"),
  );
  await confirm.click();
  expect((await response).ok()).toBeTruthy();
  await expect(dialog).toHaveCount(0);
  await expect(row).toHaveCount(0);
  expect(requests).toHaveLength(1);
});

for (const resource of [
  "server",
  "Spark",
  "role",
  "user",
  "schedule",
] as const) {
  test(`${resource} deletion uses the shared dialog and keeps errors in its existing view`, async ({
    page,
  }) => {
    await login(page);
    const requests: string[] = [];
    const paths = {
      server: "servers",
      Spark: "sparks",
      role: "roles",
      user: "users",
      schedule: "schedules",
    };
    await page.route(`**/api/v1/${paths[resource]}/*`, async (route) => {
      if (route.request().method() !== "DELETE") return route.continue();
      requests.push(route.request().url());
      await route.fulfill({
        status: 400,
        json: { error: "Deletion was rejected." },
      });
    });
    if (resource === "server") {
      await page.goto("/#servers/creative/console");
      await page
        .getByRole("button", { name: "More server actions", exact: true })
        .click();
      await page.getByRole("menuitem", { name: "Delete", exact: true }).click();
    } else if (resource === "Spark") {
      await openScreen(page, "sparks", "Sparks");
      await page
        .getByRole("button", { name: "View details for Europe", exact: true })
        .click();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Delete", exact: true })
        .click();
    } else if (resource === "role") {
      await openScreen(page, "roles", "Roles");
      await page
        .getByRole("button", {
          name: "Edit role World maintainers",
          exact: true,
        })
        .click();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Delete", exact: true })
        .click();
    } else if (resource === "user") {
      await openScreen(page, "users", "Users");
      await page
        .getByRole("button", { name: "Edit user Alex Rivera", exact: true })
        .click();
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Delete", exact: true })
        .click();
    } else {
      await openScreen(page, "schedules", "Schedules");
      await page
        .getByRole("button", {
          name: "View details for Nightly restart",
          exact: true,
        })
        .click();
      await page
        .getByRole("button", { name: "Edit schedule", exact: true })
        .click();
      await page
        .getByRole("dialog", { name: "Edit schedule", exact: true })
        .getByRole("button", { name: "Delete", exact: true })
        .click();
    }
    const dialog = page.getByRole("dialog", {
      name: `Delete ${resource}?`,
      exact: true,
    });
    await expect(dialog).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    expect(requests).toEqual([]);
    if (resource === "server") {
      await page
        .getByRole("button", { name: "More server actions", exact: true })
        .click();
      await page.getByRole("menuitem", { name: "Delete", exact: true }).click();
    } else {
      await page
        .getByRole("dialog")
        .getByRole("button", { name: "Delete", exact: true })
        .click();
    }
    await dialog.getByRole("button", { name: "Delete", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(
      page
        .getByRole("alert")
        .filter({ hasText: "Deletion was rejected." })
        .last(),
    ).toBeVisible();
    expect(requests).toHaveLength(1);
  });
}

test("danger and reinstall confirmations use the right action and lock dismissal while busy", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "console", "Console");
  const trigger = page.getByRole("button", {
    name: "More server actions",
    exact: true,
  });
  const menu = page.getByRole("menu", {
    name: "More server actions",
    exact: true,
  });
  await expect(menu).toBeHidden();
  await trigger.press("Enter");
  await expect(menu).toBeVisible();
  const kill = menu.getByRole("menuitem", { name: "Kill", exact: true });
  const reinstall = menu.getByRole("menuitem", {
    name: "Reinstall",
    exact: true,
  });
  const edit = menu.getByRole("menuitem", { name: "Edit server", exact: true });
  await expect(edit).toBeFocused();
  await expect(reinstall).toBeDisabled();
  await page.keyboard.press("ArrowDown");
  await expect(
    menu.getByRole("menuitem", { name: "Copy", exact: true }),
  ).toBeFocused();
  await prepareScreenshot(page);
  await expect(menu).toHaveScreenshot(["crops", "actions-menu.png"]);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(menu).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await expect(trigger).toBeFocused();
  await trigger.click();
  await page
    .getByRole("heading", { name: "Survival", exact: true, level: 1 })
    .click();
  await expect(menu).toBeHidden();
  await page.setViewportSize({ width: 1400, height: 1100 });
  await trigger.click();
  await kill.click();
  await expect(menu).toBeHidden();
  const danger = page.getByRole("dialog", {
    name: "Kill server: Survival",
    exact: true,
  });
  await prepareScreenshot(page);
  await expect(danger).toHaveScreenshot(["crops", "confirmation-danger.png"]);
  await danger.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Start", exact: true }),
  ).toBeEnabled();
  await trigger.click();
  await expect(kill).toBeDisabled();
  await expect(edit).toBeFocused();
  await reinstall.click();
  const dialog = page.getByRole("dialog", {
    name: "Reinstall server: Survival",
    exact: true,
  });
  const confirm = dialog.getByRole("button", {
    name: "Reinstall server",
    exact: true,
  });
  await prepareScreenshot(page);
  await expect(dialog).toHaveScreenshot([
    "crops",
    "confirmation-reinstall.png",
  ]);
  let release: () => void = () => {};
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/servers/survival/actions", async (route) => {
    await pending;
    await route.continue();
  });
  try {
    await confirm.click();
    await expect(
      dialog.getByRole("button", { name: "Close", exact: true }),
    ).toBeDisabled();
    await expect(
      dialog.getByRole("button", { name: "Cancel", exact: true }),
    ).toBeDisabled();
    await expect(
      dialog.getByRole("button", { name: "Working…", exact: true }),
    ).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
  } finally {
    release();
  }
  await expect(dialog).toHaveCount(0);
});
