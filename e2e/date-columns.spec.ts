import { test, expect, pageFixture } from "./fixtures";
import { jobFixture } from "./jobFixtures";
import { login } from "./screens";

test("date columns allow wrapping across history lists and drawers on mobile", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/v1/jobs?*", (route) =>
    route.fulfill({ json: pageFixture([jobFixture()]) }),
  );
  await page.setViewportSize({ width: 360, height: 800 });
  const cases = [
    { route: "jobs", selector: ".jobs-table .server-job-created" },
    { route: "notifications", selector: ".notification-content time" },
    { route: "audit", selector: "main .data-table tbody td:first-child" },
    {
      route: "schedules",
      selector:
        "main .data-table:has(> caption:text-is('Execution history')) tbody td:first-child",
    },
    {
      route: "schedules/nightly-restart",
      selector: "dialog .data-table tbody td:first-child",
    },
    {
      route: "servers/survival/activity",
      selector: ".activity-table tbody td:first-child",
    },
    { route: "servers/survival/backups", selector: ".backup-created" },
    {
      route: "dashboard",
      selector: "table:has(> caption:text-is('Recent backups')) time",
    },
  ];
  for (const { route, selector } of cases) {
    await test.step(route, async () => {
      await page.goto(`/#${route}`);
      const date = page.locator(selector).first();
      await expect(date).toBeVisible();
      await expect(date).toContainText(/\d/);
      await expect(date).toHaveCSS("white-space", "normal");
    });
  }
});
