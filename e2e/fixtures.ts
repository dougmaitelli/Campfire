import {
  test as base,
  expect,
  type Locator,
  type Page,
} from "@playwright/test";

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
        if (
          url.startsWith(baseURL + "/") ||
          url.startsWith("blob:" + baseURL + "/") ||
          url.startsWith("data:")
        )
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
      Array.from(document.images).map(async (image) => {
        image.loading = "eager";
        await image.decode();
      }),
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

export async function screenshot(page: Page, name: string, target?: Locator) {
  await prepareScreenshot(page);
  const viewport = page.viewportSize();
  if (!target) {
    await expect(
      page.getByRole("dialog"),
      "Capture an open drawer or dialog using its locator.",
    ).toHaveCount(0);
    const fullPage = name.endsWith("-mobile.png");
    const clip =
      fullPage && viewport
        ? await page.evaluate((width) => {
            return {
              x: 0,
              y: 0,
              width,
              height: Math.max(
                document.documentElement.scrollHeight,
                innerHeight,
              ),
            };
          }, viewport.width)
        : undefined;
    await expect(page).toHaveScreenshot(["pages", name], {
      fullPage,
      ...(clip ? { clip } : {}),
    });
    return;
  }

  const height = await target.evaluate(
    (element) => (element.closest("dialog") ?? element).scrollHeight,
  );
  const expanded = viewport !== null && height > viewport.height;
  const selects = target.locator('[role="combobox"]');
  const openSelectIndex = expanded
    ? await selects.evaluateAll((elements) =>
        elements.findIndex(
          (element) => element.getAttribute("aria-expanded") === "true",
        ),
      )
    : -1;
  const openSelect = openSelectIndex >= 0 ? selects.nth(openSelectIndex) : null;
  // Select menus close on resize; preserve the state requested by the test.
  async function restoreSelect() {
    if (openSelect) {
      await expect(openSelect).toHaveAttribute("aria-expanded", "false");
      await openSelect.click();
      await expect(openSelect).toHaveAttribute("aria-expanded", "true");
    }
  }
  try {
    if (expanded) {
      await page.setViewportSize({ ...viewport, height });
      await restoreSelect();
    }
    await target.evaluate((element) =>
      (element.closest("dialog") ?? element).scrollTo(0, 0),
    );
    await expect(target).toBeInViewport({ ratio: 1 });
    await expect(target).toHaveScreenshot(["crops", name]);
  } finally {
    if (expanded) {
      await page.setViewportSize(viewport);
      await restoreSelect();
    }
  }
}
