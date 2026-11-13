import { test, expect, screenshot } from "./fixtures";
import { login, mockServerStatus } from "./screens";

test("server console header on mobile", async ({ page }) => {
  await login(page);
  await mockServerStatus(page, "survival", () => ({
    server_id: "survival",
    state: "running",
    ready: true,
    busy: false,
    disk_quota_enforced: true,
  }));
  await page.goto("/#servers/survival/console");
  await page.setViewportSize({ width: 390, height: 844 });
  const header = page.locator(".page-header");
  await expect(header.getByText("Running", { exact: true })).toBeVisible();
  await screenshot(page, "server-console-header-mobile.png", header);
});
