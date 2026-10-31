import { test, expect, screenshot } from "./fixtures";
import { login, openServerTab, openScreen } from "./screens";

test("manual backups send the selected type and save-data restore describes its scope", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "backups", "Backups");
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  const type = page.getByRole("combobox", { name: "Backup type", exact: true });
  await type.click();
  await page
    .getByRole("option", { name: "Save data only", exact: true })
    .click();
  await screenshot(page, "backup-savedata.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await type.scrollIntoViewIfNeeded();
  await screenshot(page, "backup-savedata-mobile.png");
  const request = page.waitForRequest(
    (request) =>
      request.url().endsWith("/servers/survival/actions") &&
      request.method() === "POST",
  );
  await page
    .getByRole("button", { name: "Create backup", exact: true })
    .click();
  expect((await request).postDataJSON()).toMatchObject({
    action: "backup",
    backup_mode: "savedata",
  });
  const row = page.getByRole("row").filter({ hasText: "Save data only" });
  await expect(row).toHaveCount(1);
  await expect(
    page.getByRole("button", { name: "Create backup", exact: true }),
  ).toBeEnabled();
  let confirmation = "";
  page.once("dialog", async (dialog) => {
    confirmation = dialog.message();
    await dialog.dismiss();
  });
  await row.getByRole("button", { name: "Restore", exact: true }).click();
  expect(confirmation).toContain("keeps other installed files");
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
  await screenshot(page, "schedule-backup-mode.png");
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
  expect(await saved.json()).toMatchObject({ backup_mode: "savedata" });
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
});

test("save-data mode is unavailable when the applied Ember has no paths", async ({
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
    .getByRole("combobox", { name: "Backup type", exact: true })
    .click();
  await expect(
    page.getByRole("option", { name: "Save data only", exact: true }),
  ).toBeDisabled();
});

test("full and save-data retention settings persist independently", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "servers", "Servers");
  await page
    .getByRole("button", { name: "View details for Creative", exact: true })
    .click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Edit server", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Edit server", exact: true });
  for (const name of ["Next: Resources & ports", "Next: Backups"]) {
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
  await screenshot(page, "backup-retention-modes.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "backup-retention-modes-mobile.png");
  for (const name of ["Next: Access", "Next: Review", "Save changes"]) {
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
