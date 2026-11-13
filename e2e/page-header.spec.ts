import { test, expect } from "./fixtures";
import { login } from "./screens";

const creationPages = [
  {
    id: "schedules",
    title: "Schedules",
    action: "+ Create schedule",
    compact: "Create",
  },
  {
    id: "servers",
    title: "Servers",
    action: "+ Create server",
    compact: "Create",
  },
  { id: "sparks", title: "Sparks", action: "+ Add Spark", compact: "Add" },
  {
    id: "embers",
    title: "Embers",
    action: "+ Create Ember",
    compact: "Create",
  },
  { id: "users", title: "Users", action: "+ Create user", compact: "Create" },
  { id: "roles", title: "Roles", action: "+ Create role", compact: "Create" },
];

for (const width of [320, 390, 640]) {
  test(`shared mobile headers keep their primary action beside the title at ${width}px`, async ({
    page,
  }) => {
    await login(page);
    await page.setViewportSize({ width, height: 844 });
    for (const screen of creationPages) {
      await page.goto(`/#${screen.id}`);
      const header = page.locator(".page-header");
      await expect(
        header.getByRole("heading", { name: screen.title, exact: true }),
      ).toBeVisible();
      const button = header.getByRole("button", {
        name: screen.action,
        exact: true,
      });
      await expect(button).toBeVisible();
      await page.evaluate(() => document.fonts.ready);
      await expect(
        button.locator(".page-header-action-compact-label"),
      ).toHaveText(screen.compact);
      await expect(
        button.locator(".page-header-action-compact-label"),
      ).toBeVisible();
      await expect(button.locator(".page-header-action-label")).toBeHidden();
      const title = await header.locator(".page-header-title").boundingBox();
      const actions = await header.locator(".page-actions").boundingBox();
      const description = await header
        .locator(".page-description")
        .boundingBox();
      const bounds = await header.boundingBox();
      const touchTarget = await button.boundingBox();
      if (!title || !actions || !description || !bounds || !touchTarget)
        throw new Error(`Missing header layout on ${screen.id}`);
      expect(
        Math.abs(title.y + title.height / 2 - actions.y - actions.height / 2),
      ).toBeLessThan(1);
      expect(title.x + title.width).toBeLessThanOrEqual(actions.x);
      expect(description.y).toBeGreaterThanOrEqual(
        Math.max(title.y + title.height, actions.y + actions.height),
      );
      expect(actions.x + actions.width).toBeLessThanOrEqual(
        bounds.x + bounds.width + 1,
      );
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width);
      expect(touchTarget.height).toBeGreaterThanOrEqual(44);
      if (screen.id === "embers") {
        await expect(
          header.getByRole("button", { name: "+ Import Ember", exact: true }),
        ).toBeHidden();
        await header
          .getByRole("button", { name: "Actions", exact: true })
          .click();
        await expect(
          header.getByRole("menuitem", { name: "+ Import Ember", exact: true }),
        ).toBeVisible();
        await page.keyboard.press("Escape");
      }
    }
  });
}

test("desktop headers preserve their full action labels and visible secondary actions", async ({
  page,
}) => {
  await login(page);
  for (const screen of creationPages) {
    await page.goto(`/#${screen.id}`);
    const header = page.locator(".page-header");
    const button = header.getByRole("button", {
      name: screen.action,
      exact: true,
    });
    await expect(button).toBeVisible();
    await expect(button.locator(".page-header-action-label")).toBeVisible();
    await expect(
      button.locator(".page-header-action-compact-label"),
    ).toBeHidden();
    await expect(button.locator(".page-header-primary-icon")).toBeHidden();
    if (screen.id === "embers") {
      await expect(
        header.getByRole("button", { name: "+ Import Ember", exact: true }),
      ).toBeVisible();
      await expect(header.locator(".page-header-secondary-menu")).toBeHidden();
    }
  }
});
