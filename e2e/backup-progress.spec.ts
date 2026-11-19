import { test, expect, screenshot } from "./fixtures";
import { login, mockServerStatus, openScreen } from "./screens";

test("measured archive progress survives navigation and never treats bytes as a commit", async ({
  page,
}) => {
  await login(page);
  const now = await page.evaluate(() => Date.now());
  await page.clock.install({ time: new Date(now) });
  const base = (await (
    await page.request.get("/api/v1/servers/survival/status")
  ).json()) as Record<string, unknown>;
  let state = "running";
  let progress = {
    phase: "archiving",
    input_bytes: 10485760,
    output_bytes: 1048576,
    input_total: 0,
    entries: 42,
    updated_at: new Date(now).toISOString(),
  };
  const status = () => ({
    ...base,
    operation_id: "measured-backup",
    operation: "backup",
    operation_state: state,
    busy: state === "running",
    file_system_busy: state === "running",
    progress: state === "running" ? "Creating backup" : "Failed",
    error: state === "failed" ? "Archive publication failed" : "",
    work_progress: progress,
  });
  await mockServerStatus(page, "survival", status);
  await page.route("**/api/v1/servers/survival/status", (route) =>
    route.fulfill({ json: status() }),
  );
  await openScreen(page, "servers", "Servers");
  await expect(
    page.getByRole("group", { name: "Creating archive", exact: true }),
  ).toBeVisible();
  await page.goto("/#servers/survival/backups");
  let group = page.getByRole("group", {
    name: "Creating archive",
    exact: true,
  });
  await expect(group).toBeVisible();
  await expect(group.getByRole("progressbar")).not.toHaveAttribute("value");
  await screenshot(page, "backup-progress.png", group);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "backup-progress-mobile.png", group);
  await page.goto("/#servers/survival/activity");
  await expect(group).toBeVisible();
  progress = {
    ...progress,
    phase: "extracting",
    input_bytes: 1048576,
    input_total: 1048576,
    output_bytes: 10485760,
    updated_at: new Date(now).toISOString(),
  };
  group = page.getByRole("group", {
    name: "Extracting and validating archive",
    exact: true,
  });
  await expect(group).toBeVisible();
  await expect(group).toContainText(
    "Measured total reached; the operation is still in progress.",
  );
  await page.clock.runFor(35000);
  await expect(group).toContainText("No recent measurements");
  await expect(group.getByRole("progressbar")).toHaveCount(0);
  state = "failed";
  await page.goto("/#servers/survival/backups");
  await expect(
    page.getByText("Archive publication failed", { exact: true }),
  ).toBeVisible();
  await expect(group).toHaveCount(0);
});

test("catalog polling replaces synchronization measurements across navigation", async ({
  page,
}) => {
  await login(page);
  let input = 1024;
  await page.route("**/api/v1/backups", async (route) => {
    const response = await route.fetch();
    const rows = (await response.json()) as Record<string, unknown>[];
    rows[0] = {
      ...rows[0],
      state: "syncing",
      remote: false,
      work_progress: {
        phase: "uploading",
        input_bytes: input,
        input_total: 4096,
        output_bytes: 0,
        entries: 0,
        updated_at: new Date().toISOString(),
      },
    };
    await route.fulfill({ response, json: rows });
  });
  await openScreen(page, "backups", "Backups");
  let group = page.getByRole("group", {
    name: "Streaming to storage",
    exact: true,
  });
  await expect(group.getByRole("progressbar")).toHaveAttribute("value", "1024");
  input = 4096;
  await expect(group.getByRole("progressbar")).toHaveAttribute("value", "4096");
  await expect(group).toContainText(
    "Measured total reached; the operation is still in progress.",
  );
  await openScreen(page, "servers", "Servers");
  await openScreen(page, "backups", "Backups");
  group = page.getByRole("group", {
    name: "Streaming to storage",
    exact: true,
  });
  await expect(group.getByRole("progressbar")).toHaveAttribute("value", "4096");
  await expect(group).toHaveCount(1);
});

test("a user-started restore shows measurements inside its locked dialog", async ({
  page,
}) => {
  await login(page);
  const consoleErrors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") consoleErrors.push(message.text());
  });
  let observed: Record<string, unknown> | undefined;
  let release = false;
  const now = await page.evaluate(() => Date.now());
  await page.route("**/api/v1/servers/creative/actions", async (route) => {
    const response = await route.fetch();
    const status = (await response.json()) as Record<string, unknown>;
    observed = {
      ...status,
      operation: "restore",
      operation_state: "running",
      busy: true,
      file_system_busy: true,
      work_progress: {
        phase: "extracting",
        input_bytes: 1024,
        input_total: 4096,
        output_bytes: 8192,
        entries: 12,
        updated_at: new Date(now).toISOString(),
      },
    };
    await route.fulfill({ response, json: observed });
  });
  await page.route("**/api/v1/servers/creative/status", async (route) => {
    if (observed && !release) await route.fulfill({ json: observed });
    else await route.continue();
  });
  await page.goto("/#servers/creative/backups");
  await page
    .getByRole("row")
    .filter({ hasText: "mock-nightly.tar.gz" })
    .getByRole("button", { name: "Restore", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Restore backup for Creative?",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Restore", exact: true }).click();
  await expect(
    dialog.getByRole("group", {
      name: "Extracting and validating archive",
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.locator(".work-progress")).toHaveCount(1);
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeDisabled();
  await expect(page.locator("p div.work-progress")).toHaveCount(0);
  // This dialog is intentionally busy, unlike the settled-page screenshot helper.
  await page.evaluate(() => document.fonts.ready);
  await expect(dialog).toHaveScreenshot([
    "crops",
    "backup-restore-progress.png",
  ]);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(dialog).toHaveScreenshot([
    "crops",
    "backup-restore-progress-mobile.png",
  ]);
  release = true;
  await expect(dialog).toHaveCount(0);
  expect(
    consoleErrors.filter((message) =>
      /cannot contain|cannot be a descendant|validateDOMNesting/.test(message),
    ),
  ).toEqual([]);
});
