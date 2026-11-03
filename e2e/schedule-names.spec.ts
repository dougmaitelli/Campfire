import { test, expect, screenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("new schedule names follow selections and persist without being typed", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "schedules", "Schedules");
  await page
    .getByRole("button", { name: "+ Create schedule", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Create schedule",
    exact: true,
  });
  const name = drawer.getByRole("textbox", { name: "Name", exact: true });
  await expect(name).toHaveValue("Creative - Daily Backup at 3am");
  await drawer.getByRole("combobox", { name: "Server", exact: true }).click();
  await page.getByRole("option", { name: "Survival", exact: true }).click();
  await expect(name).toHaveValue("Survival - Daily Backup at 3am");
  await drawer.getByLabel("Cron schedule", { exact: true }).fill("0 5 * * *");
  await expect(name).toHaveValue("Survival - Daily Backup at 5am");
  await expect(
    drawer.getByRole("button", { name: "Save schedule", exact: true }),
  ).toBeEnabled();
  await screenshot(page, "schedule-generated-name.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await name.scrollIntoViewIfNeeded();
  await screenshot(page, "schedule-generated-name-mobile.png");
  await page.setViewportSize({ width: 1400, height: 900 });
  await drawer.getByRole("combobox", { name: "Action", exact: true }).click();
  await page.getByRole("option", { name: "Restart", exact: true }).click();
  await drawer.getByRole("button", { name: "Hourly", exact: true }).click();
  await expect(name).toHaveValue("Survival - Hourly Restart");
  await drawer
    .getByRole("combobox", { name: "Time zone", exact: true })
    .click();
  await page
    .getByRole("option", { name: "America/Los Angeles", exact: true })
    .click();
  await expect(name).toHaveValue(
    "Survival - Hourly Restart (America/Los_Angeles)",
  );
  await drawer.getByRole("combobox", { name: "Timing", exact: true }).click();
  await page.getByRole("option", { name: "Run once", exact: true }).click();
  await drawer.getByLabel("Run at", { exact: true }).fill("2026-10-23T14:30");
  const generatedName = "Survival - Restart - Oct 23, 2026, 2:30 PM (UTC)";
  await expect(name).toHaveValue(generatedName);
  await drawer
    .getByRole("checkbox", { name: "Enabled", exact: true })
    .uncheck();
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
    name: generatedName,
    action: "restart",
    server_id: "survival",
    cron_expression: "",
    enabled: false,
  });
  await page
    .getByRole("button", {
      name: `View details for ${generatedName}`,
      exact: true,
    })
    .click();
  await page
    .getByRole("dialog", { name: generatedName, exact: true })
    .getByRole("button", { name: "Edit schedule", exact: true })
    .click();
  const editing = page.getByRole("dialog", {
    name: "Edit schedule",
    exact: true,
  });
  await editing.getByRole("combobox", { name: "Action", exact: true }).click();
  await page.getByRole("option", { name: "Stop", exact: true }).click();
  await expect(
    editing.getByRole("textbox", { name: "Name", exact: true }),
  ).toHaveValue(generatedName);
});

test("custom schedule names remain unchanged when other settings change", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "schedules", "Schedules");
  await page
    .getByRole("button", { name: "+ Create schedule", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Create schedule",
    exact: true,
  });
  const name = drawer.getByRole("textbox", { name: "Name", exact: true });
  await name.fill("World maintenance");
  await drawer.getByRole("combobox", { name: "Server", exact: true }).click();
  await page.getByRole("option", { name: "Survival", exact: true }).click();
  await drawer.getByRole("combobox", { name: "Action", exact: true }).click();
  await page.getByRole("option", { name: "Restart", exact: true }).click();
  await drawer.getByRole("button", { name: "Weekly", exact: true }).click();
  await drawer
    .getByRole("combobox", { name: "Time zone", exact: true })
    .click();
  await page.getByRole("option", { name: "Europe/Paris", exact: true }).click();
  await expect(name).toHaveValue("World maintenance");
  await name.fill("");
  await expect(
    drawer.getByRole("button", { name: "Save schedule", exact: true }),
  ).toBeDisabled();
});

test("recurring schedules save the selected time zone and readable name", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "schedules", "Schedules");
  await page
    .getByRole("button", { name: "+ Create schedule", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Create schedule",
    exact: true,
  });
  await drawer.getByLabel("Cron schedule", { exact: true }).fill("0 5 * * *");
  const zone = drawer.getByRole("combobox", {
    name: "Time zone",
    exact: true,
  });
  await expect(
    drawer.getByRole("button", { name: "Save schedule", exact: true }),
  ).toBeEnabled();
  await zone.click();
  await expect(
    page.getByRole("option", { name: "UTC", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("listbox")).toHaveScreenshot(
    "schedule-time-zone.png",
  );
  await page.keyboard.press("Escape");
  await page.setViewportSize({ width: 390, height: 844 });
  await zone.scrollIntoViewIfNeeded();
  await zone.click();
  await expect(page.getByRole("listbox")).toBeVisible();
  await page.getByRole("option", { name: "Europe/Paris", exact: true }).click();
  await expect(zone).toHaveText("Europe/Paris");
  const generatedName = "Creative - Daily Backup at 5am (Europe/Paris)";
  await expect(
    drawer.getByRole("textbox", { name: "Name", exact: true }),
  ).toHaveValue(generatedName);
  await drawer
    .getByRole("checkbox", { name: "Enabled", exact: true })
    .uncheck();
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
    name: generatedName,
    cron_expression: "0 5 * * *",
    time_zone: "Europe/Paris",
  });
  await page.reload();
  await page
    .getByRole("button", {
      name: `View details for ${generatedName}`,
      exact: true,
    })
    .click();
  await expect(
    page
      .getByRole("dialog", { name: generatedName, exact: true })
      .getByText("Europe/Paris", { exact: true }),
  ).toBeVisible();
});
