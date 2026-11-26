import { DiskAccounting } from "../web/src/contracts/domains";
import type { MonitoringSnapshot } from "../web/src/api/monitoring";
import { test, expect } from "./fixtures";
import { login, openServerTab } from "./screens";

test("current disk accounting identifies native, failed, and legacy readings without disk history", async ({
  page,
}) => {
  let method: DiskAccounting | undefined = DiskAccounting.Project;
  let failed = false;
  await page.route("**/api/v1/monitoring", async (route) => {
    const response = await route.fetch();
    const snapshot = (await response.json()) as MonitoringSnapshot;
    const server = snapshot.servers.find((item) => item.id === "survival");
    if (!server) throw new Error("Missing survival fixture");
    server.usage.disk_bytes = 8388608;
    server.usage.disk_sampled_at = snapshot.collected_at;
    server.usage.disk_age_seconds = failed ? 120 : 0;
    server.usage.disk_error = failed
      ? "Counter unavailable; last reading may be stale."
      : "";
    server.usage.disk_quota_error = "";
    if (method) server.usage.disk_accounting = method;
    else delete server.usage.disk_accounting;
    await route.fulfill({ response, json: snapshot });
  });
  await login(page);
  await openServerTab(page, "console", "Console");
  const reading = page.locator(".metric-tiles .usage-reading").filter({
    has: page.getByText("Game data", { exact: true }),
  });
  await expect(reading).toContainText("8 MiB");
  await expect(reading).toHaveAttribute(
    "title",
    /Filesystem project-quota usage/,
  );
  await expect(page.locator(".resource-graphs .resource-graph")).toHaveCount(2);
  await expect(page.locator(".resource-graphs")).not.toContainText("Game data");

  failed = true;
  await page.reload();
  await expect(reading).toContainText("8 MiB");
  await expect(reading).toContainText("stale");
  await expect(reading).toHaveAttribute(
    "title",
    /Counter unavailable.*Filesystem project-quota usage/,
  );
  await expect(reading.getByRole("meter")).toHaveCount(0);

  failed = false;
  method = undefined;
  await page.reload();
  await expect(reading).toHaveAttribute(
    "title",
    /Logical game-data file sizes from a bounded scan/,
  );
  await expect(reading).not.toContainText("stale");
  await expect(reading.getByRole("meter")).toHaveCount(1);
});
