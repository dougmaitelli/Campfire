import { test, expect } from "./fixtures";
import { login } from "./screens";

test.use({
  isMobile: true,
  hasTouch: true,
  deviceScaleFactor: 3,
  locale: "en-US",
});

for (const width of [360, 390, 412]) {
  test(`file dates wrap and remain visible at ${width}px`, async ({ page }) => {
    await login(page);
    await page.setViewportSize({ width, height: 915 });
    const modified = "2026-12-28T23:59:59Z";
    await page.route("**/api/v1/servers/survival/files", (route) =>
      route.fulfill({
        json: {
          server_id: "survival",
          path: "/",
          entries: [
            {
              name: "world",
              kind: "directory",
              size: 0,
              modified_at: modified,
            },
            {
              name: "long-world-backup-filename-that-needs-to-fit-on-a-small-screen.tar.gz",
              kind: "file",
              size: 123456789,
              modified_at: modified,
            },
            {
              name: "server.properties",
              kind: "file",
              size: 1234,
              modified_at: modified,
            },
          ],
        },
      }),
    );
    await page.goto("/#servers/survival/files");
    const browser = page.getByRole("region", {
      name: "File browser",
      exact: true,
    });
    const table = browser.getByRole("table");
    const dates = table.locator("time");
    await expect(dates).toHaveCount(3);
    await expect(
      table.getByRole("columnheader", { name: "Modified", exact: true }),
    ).toBeVisible();
    await expect(table.locator("td.table-date").first()).toHaveCSS(
      "white-space",
      "normal",
    );
    for (const date of await dates.all()) {
      await expect(date).toBeVisible();
      await expect(date).toHaveAttribute("datetime", modified);
    }
    expect(
      await dates
        .first()
        .evaluate((element) => element.getClientRects().length),
    ).toBeGreaterThan(1);
    await expect(
      browser.getByRole("checkbox", { name: "Select all items", exact: true }),
    ).toBeVisible();
    await expect(
      browser.getByRole("button", {
        name: "Download server.properties",
        exact: true,
      }),
    ).toBeVisible();
    if (width === 412) {
      await page.setViewportSize({ width: 1400, height: 900 });
      await expect(table.locator("td.table-date").first()).toHaveCSS(
        "white-space",
        "nowrap",
      );
    }
  });
}
