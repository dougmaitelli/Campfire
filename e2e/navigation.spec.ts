import { test, expect } from "./fixtures";
import { login } from "./screens";

for (const mobile of [false, true]) {
  test(`returning to Servers after opening a console from Overview shows the list (${mobile ? "mobile" : "desktop"})`, async ({
    page,
  }) => {
    await login(page);
    if (mobile) await page.setViewportSize({ width: 390, height: 844 });
    async function navigate(label: string) {
      if (mobile)
        await page
          .getByRole("button", { name: "Open navigation", exact: true })
          .click();
      await page
        .getByRole("navigation", { name: "Main navigation", exact: true })
        .getByRole("link", { name: label, exact: true })
        .click();
      await expect(
        page.getByRole("heading", { name: label, exact: true, level: 1 }),
      ).toBeVisible();
    }

    for (const [destination, heading] of [
      ["schedules", "Schedules"],
      ["sparks", "Sparks"],
    ] as const) {
      await navigate("Overview");
      await page
        .getByRole("button", { name: "Open Survival", exact: true })
        .click();
      await expect(page).toHaveURL(/#servers\/survival\/console$/);
      await expect(
        page.getByRole("region", { name: "Console output", exact: true }),
      ).toBeVisible();

      await navigate(heading);
      await navigate("Servers");
      await expect(page).toHaveURL(/#servers$/);
      await expect(
        page.getByRole("table", { name: "Game servers", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("region", { name: "Console output", exact: true }),
      ).toHaveCount(0);

      await page.goBack();
      await expect(page).toHaveURL(new RegExp(`#${destination}$`));
      await expect(
        page.getByRole("heading", { name: heading, exact: true, level: 1 }),
      ).toBeVisible();
      await page.goForward();
      await expect(page).toHaveURL(/#servers$/);
      await expect(
        page.getByRole("table", { name: "Game servers", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("region", { name: "Console output", exact: true }),
      ).toHaveCount(0);
    }
  });
}
