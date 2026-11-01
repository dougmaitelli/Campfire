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
  await expect(name).toHaveValue(
    "Creative · Backup · Every day at 03:00 (UTC)",
  );
  await drawer.getByRole("combobox", { name: "Server", exact: true }).click();
  await page.getByRole("option", { name: "Survival", exact: true }).click();
  await expect(name).toHaveValue(
    "Survival · Backup · Every day at 03:00 (UTC)",
  );
  await drawer.getByRole("combobox", { name: "Action", exact: true }).click();
  await page.getByRole("option", { name: "Restart", exact: true }).click();
  await drawer.getByRole("button", { name: "Hourly", exact: true }).click();
  await expect(name).toHaveValue(
    "Survival · Restart · Every hour, on the hour (UTC)",
  );
  await drawer
    .getByLabel("Time zone", { exact: true })
    .fill("America/Los_Angeles");
  await expect(name).toHaveValue(
    "Survival · Restart · Every hour, on the hour (America/Los_Angeles)",
  );
  await drawer.getByRole("combobox", { name: "Timing", exact: true }).click();
  await page.getByRole("option", { name: "Run once", exact: true }).click();
  await drawer.getByLabel("Run at", { exact: true }).fill("2026-10-23T14:30");
  const generatedName = "Survival · Restart · Oct 23, 2026, 2:30 PM (UTC)";
  await expect(name).toHaveValue(generatedName);
  await screenshot(page, "schedule-generated-name.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await name.scrollIntoViewIfNeeded();
  await screenshot(page, "schedule-generated-name-mobile.png");
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
  await drawer.getByLabel("Time zone", { exact: true }).fill("Europe/Paris");
  await expect(name).toHaveValue("World maintenance");
  await name.fill("");
  await expect(
    drawer.getByRole("button", { name: "Save schedule", exact: true }),
  ).toBeDisabled();
});
