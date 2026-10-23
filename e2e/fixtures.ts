import { test as base, expect, type Page } from "@playwright/test";

export const test = base.extend<{ guard: void }>({
  guard: [
    async ({ page, request, baseURL }, use) => {
      expect(
        (
          await request.post("/__test/reset", {
            headers: { "X-Campfire-E2E": "reset" },
          })
        ).ok(),
      ).toBeTruthy();
      const errors: string[] = [];
      page.on("pageerror", (error) => errors.push(error.message));
      page.on("response", (response) => {
        if (response.status() >= 500)
          errors.push(`HTTP ${response.status()}: ${response.url()}`);
      });
      await page.route("**/*", async (route) => {
        const url = route.request().url();
        if (url.startsWith(baseURL + "/") || url.startsWith("data:"))
          return route.continue();
        errors.push(`Unexpected external request: ${url}`);
        await route.abort();
      });
      await page.clock.setFixedTime(new Date("2026-10-22T12:00:00Z"));
      await use();
      expect(
        errors,
        "Browser exceptions and unexpected external requests",
      ).toEqual([]);
    },
    { auto: true },
  ],
});
export { expect };

export async function prepareScreenshot(page: Page) {
  if (await page.locator(".notification-link").count()) {
    await expect(page.locator(".notification-link")).toHaveAttribute(
      "aria-label",
      /^Notifications, \d+ unread$/,
    );
  }
  await page.evaluate(() => document.fonts.ready);
  await page.evaluate(async () => {
    await Promise.all(
      Array.from(document.images).map((image) => image.decode()),
    );
    await Promise.all(
      document
        .getAnimations()
        .filter(
          (animation) => animation.effect?.getTiming().iterations !== Infinity,
        )
        .map((animation) => animation.finished.catch(() => {})),
    );
  });
  await expect(page.locator('[aria-busy="true"]')).toHaveCount(0);
  await page.mouse.move(0, 0);
}

export async function screenshot(page: Page, name: string) {
  await prepareScreenshot(page);
  await expect(page).toHaveScreenshot(name);
}
