import { test, expect, screenshot } from "./fixtures";
import { login, openServerTab, openScreen, openServerEditor } from "./screens";

test("manual backups send the selected type and save-data restore describes its scope", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "backups", "Backups");
  await expect(
    page.getByRole("button", { name: "Create backup", exact: true }),
  ).toBeEnabled();
  for (const restore of await page
    .getByRole("button", { name: "Restore", exact: true })
    .all()) {
    await expect(restore).toBeEnabled();
  }
  await expect(
    page.getByRole("combobox", { name: "Backup type", exact: true }),
  ).toHaveCount(0);
  const create = page.getByRole("button", {
    name: "Create backup",
    exact: true,
  });
  await create.focus();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("menuitem", { name: "Full backup", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(
    page.getByRole("menuitem", { name: "Save data only", exact: true }),
  ).toBeFocused();
  await screenshot(page, "backup-savedata.png");
  await page.keyboard.press("Escape");
  await page.setViewportSize({ width: 390, height: 844 });
  const backups = page.getByRole("table", {
    name: "Server backups",
    exact: true,
  });
  const firstBackup = backups
    .getByRole("row")
    .filter({ hasText: "mock-nightly" });
  for (const width of [320, 390, 600, 768]) {
    await page.setViewportSize({ width, height: 844 });
    await create.click();
    await expect(
      page.getByRole("menu", { name: "Create backup", exact: true }),
    ).toBeInViewport({ ratio: 1 });
    await page.keyboard.press("Escape");
    const name = firstBackup.getByText("mock-nightly.tar.gz", { exact: true });
    await name.scrollIntoViewIfNeeded();
    await expect(name).toBeInViewport({ ratio: 1 });
    for (const action of await firstBackup.getByRole("button").all())
      await expect(action).toBeInViewport({ ratio: 1 });
    await expect(firstBackup.getByRole("link")).toBeInViewport({ ratio: 1 });
  }
  await page.setViewportSize({ width: 390, height: 844 });
  await create.click();
  await screenshot(page, "backup-savedata-mobile.png");
  await page.keyboard.press("Escape");
  const actions = firstBackup.getByRole("button", {
    name: /^Backup actions for /,
  });
  await actions.click();
  await expect(
    page.getByRole("menuitem", { name: "Rename backup", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  const request = page.waitForRequest(
    (request) =>
      request.url().endsWith("/servers/survival/actions") &&
      request.method() === "POST",
  );
  await page
    .getByRole("button", { name: "Create backup", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Save data only", exact: true })
    .click();
  expect((await request).postDataJSON()).toMatchObject({
    action: "backup",
    backup_mode: "savedata",
  });
  const row = page.getByRole("row").filter({ hasText: "Save data only" });
  await expect(row).toHaveCount(1);
  await row.getByRole("button", { name: /^Backup actions for / }).click();
  await page
    .getByRole("menuitem", { name: "Rename backup", exact: true })
    .click();
  const rename = page.getByRole("dialog", {
    name: "Rename backup",
    exact: true,
  });
  await rename
    .getByRole("textbox", { name: "Backup name", exact: true })
    .fill("Save data before game update");
  await rename.getByRole("textbox").press("Enter");
  await expect(rename).toHaveCount(0);
  await expect(row).toContainText("Save data before game update");
  await expect(
    page.getByRole("button", { name: "Create backup", exact: true }),
  ).toBeEnabled();
  const status = (await (
    await page.request.get("/api/v1/servers/survival/status")
  ).json()) as { state: string };
  expect(status.state).toBe("running");
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(
    row.getByRole("button", { name: "Restore", exact: true }),
  ).toBeEnabled();
  await row.getByRole("button", { name: "Restore", exact: true }).click();
  const confirmation = page.getByRole("dialog", {
    name: "Restore backup for Survival?",
    exact: true,
  });
  await expect(confirmation).toContainText("keeps other installed files");
  await expect(confirmation).toContainText("The server will remain stopped");
  await page.setViewportSize({ width: 1400, height: 900 });
  await screenshot(page, "restore-savedata-confirm.png", confirmation);
  await confirmation
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
});

test("backup schedules retain their explicit type", async ({ page }) => {
  await login(page);
  await openScreen(page, "schedules", "Schedules");
  await page.getByRole("button", { name: /Create schedule/ }).click();
  const drawer = page.getByRole("dialog", {
    name: "Create schedule",
    exact: true,
  });
  await drawer
    .getByRole("textbox", { name: "Name", exact: true })
    .fill("Save-data snapshot");
  await drawer.getByRole("combobox", { name: "Action", exact: true }).click();
  await page.getByRole("option", { name: "Backup", exact: true }).click();
  await drawer
    .getByRole("combobox", { name: "Backup type", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Save data only", exact: true })
    .click();
  await drawer
    .getByRole("checkbox", { name: "Stop server during backup", exact: true })
    .check();
  await screenshot(page, "schedule-backup-mode.png", drawer);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "schedule-backup-mode-mobile.png", drawer);
  const response = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/schedules") &&
      response.request().method() === "POST",
  );
  await drawer
    .getByRole("button", { name: "Save schedule", exact: true })
    .click();
  const saved = await response;
  expect(saved.ok()).toBeTruthy();
  expect(await saved.json()).toMatchObject({
    backup_mode: "savedata",
    backup_stop_server: true,
  });
  await page.reload();
  await page
    .getByRole("button", {
      name: "View details for Save-data snapshot",
      exact: true,
    })
    .click();
  await expect(
    page
      .getByRole("dialog", { name: "Save-data snapshot", exact: true })
      .getByText("Save data only", { exact: true }),
  ).toBeVisible();
  const stopOption = page
    .getByRole("dialog", { name: "Save-data snapshot", exact: true })
    .getByRole("checkbox", {
      name: "Stop server during backup",
      exact: true,
    });
  await expect(stopOption).toBeChecked();
  await expect(stopOption).toBeDisabled();
});

test("backup menu only offers supported types and creates a full backup", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/v1/servers/survival", async (route) => {
    const response = await route.fetch();
    const server = await response.json();
    await route.fulfill({
      response,
      json: { ...server, savedata_available: false },
    });
  });
  await openServerTab(page, "backups", "Backups");
  await page
    .getByRole("button", { name: "Create backup", exact: true })
    .click();
  await expect(
    page.getByRole("menuitem", { name: "Save data only", exact: true }),
  ).toHaveCount(0);
  const request = page.waitForRequest(
    (request) =>
      request.method() === "POST" &&
      request.url().endsWith("/servers/survival/actions"),
  );
  await page
    .getByRole("menuitem", { name: "Full backup", exact: true })
    .click();
  expect((await request).postDataJSON()).toMatchObject({
    action: "backup",
    backup_mode: "full",
  });
});

test("full and save-data retention settings persist independently", async ({
  page,
}) => {
  await login(page);
  const drawer = await openServerEditor(page);
  for (const name of [
    "Next: Variables",
    "Next: Resources & ports",
    "Next: Backups",
  ]) {
    await drawer.getByRole("button", { name, exact: true }).click();
  }
  const full = drawer.getByRole("group", { name: "Full server", exact: true });
  const savedata = drawer.getByRole("group", {
    name: "Save data only",
    exact: true,
  });
  await full
    .getByRole("spinbutton", { name: "Local copies to keep", exact: true })
    .fill("3");
  await savedata
    .getByRole("spinbutton", { name: "Local copies to keep", exact: true })
    .fill("12");
  await savedata
    .getByRole("spinbutton", { name: "Local maximum age (days)", exact: true })
    .fill("7");
  await screenshot(page, "backup-retention-modes.png", drawer);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "backup-retention-modes-mobile.png", drawer);
  for (const name of [
    "Next: Integrations",
    "Next: Access",
    "Next: Review",
    "Save changes",
  ]) {
    await drawer.getByRole("button", { name, exact: true }).click();
  }
  await page
    .getByRole("dialog", { name: "Save server changes?", exact: true })
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(drawer).toHaveCount(0);
  const response = await page.request.get(
    "/api/v1/servers/creative/backup-policy",
  );
  expect(response.ok()).toBeTruthy();
  expect(await response.json()).toMatchObject({
    policy: {
      local_keep_last: 3,
      savedata: { local_keep_last: 12, local_max_age_days: 7 },
    },
  });
});
