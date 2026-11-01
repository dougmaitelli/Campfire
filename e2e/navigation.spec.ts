import { test, expect } from "./fixtures";
import { login, openServerTab } from "./screens";

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

test("unsaved file edits can block or allow navigation", async ({ page }) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  const editor = page.getByRole("textbox", {
    name: "/server.properties",
    exact: true,
  });
  const original = await editor.inputValue();
  const changed = original + "\n# unsaved changes";
  await editor.fill(changed);
  await expect(
    page.getByRole("status").filter({ hasText: "Unsaved changes" }),
  ).toBeVisible();
  const schedules = page
    .getByRole("navigation", { name: "Main navigation", exact: true })
    .getByRole("link", { name: "Schedules", exact: true });
  page.once("dialog", (dialog) => void dialog.dismiss());
  await schedules.click();
  await expect(page).toHaveURL(/#servers\/survival\/files$/);
  await expect(editor).toHaveValue(changed);

  page.once("dialog", (dialog) => void dialog.accept());
  await schedules.click();
  await expect(page).toHaveURL(/#schedules$/);
  await expect(
    page.getByRole("heading", { name: "Schedules", exact: true, level: 1 }),
  ).toBeVisible();
  await page.goto("/#servers/survival/files");
  await page
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  await expect(editor).toHaveValue(original);
});
