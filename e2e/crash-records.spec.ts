import { test, expect } from "./fixtures";
import { login } from "./screens";
import { CrashReason } from "../web/src/contracts/domains";
import type { CrashRecord } from "../web/src/api/crashes";
import type { ActivityDetails } from "../web/src/api/serverActivity";

test("crash evidence is retained under Activity Alerts across page reloads", async ({
  page,
}) => {
  await login(page);
  const record: CrashRecord = {
    id: "a".repeat(64),
    container_id: "b".repeat(64),
    exited_at: Math.floor(Date.now() / 1000),
    exit_code: 137,
    reason: CrashReason.OutOfMemory,
    console: "[server] fatal allocation failure — private crash evidence",
    console_unavailable: false,
    console_truncated: true,
  };
  for (let replay = 0; replay < 2; replay++) {
    const response = await page.request.post("/__test/crash", {
      headers: { "X-Campfire-E2E": "crash" },
      data: { server_id: "survival", record },
    });
    expect(response.status()).toBe(204);
  }
  await page.goto("/#servers/survival/activity?kind=alert&search=crashed");
  const activity = page.getByRole("region", { name: "Activity", exact: true });
  await expect(activity.locator("tbody tr")).toHaveCount(1);
  await expect(activity).not.toContainText(record.console!);
  await activity.getByRole("button", { name: /crashed/i }).click();
  const dialog = page.getByRole("dialog", { name: /crashed/i });
  await expect(dialog.getByText("137", { exact: true })).toBeVisible();
  await expect(
    dialog.getByText("Out of memory", { exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByText(record.console!, { exact: true }),
  ).toBeVisible();
  await expect(
    dialog.getByText(/Only the most recent 200 lines/),
  ).toBeVisible();
  expect((await dialog.locator("dt").allTextContents()).slice(0, 2)).toEqual([
    "Activity type",
    "Container ID",
  ]);
  await expect(dialog.getByText("Started", { exact: true })).toHaveCount(0);
  await expect(dialog.getByText("Recorded", { exact: true })).toHaveCount(0);
  await page.reload();
  await expect(
    dialog.getByText(record.console!, { exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(dialog.getByText("137", { exact: true })).toBeVisible();
  expect(
    await dialog.evaluate(
      (element) => element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true);
  const response = await page.request.get(
    `/api/v1/servers/survival/activity/crash:${record.id}`,
  );
  expect(response.ok()).toBeTruthy();
  const details = (await response.json()) as ActivityDetails;
  expect(details.crash?.console).toBe(record.console);
});
