import { test, expect, prepareScreenshot } from "./fixtures";
import { login, openServerTab } from "./screens";

test("danger and reinstall confirmations use the right action and lock dismissal while busy", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "console", "Console");
  const trigger = page.getByRole("button", {
    name: "More server actions",
    exact: true,
  });
  const menu = page.getByRole("menu", {
    name: "More server actions",
    exact: true,
  });
  await expect(menu).toBeHidden();
  await trigger.press("Enter");
  await expect(menu).toBeVisible();
  const kill = menu.getByRole("menuitem", { name: "Kill", exact: true });
  const reinstall = menu.getByRole("menuitem", {
    name: "Reinstall",
    exact: true,
  });
  const edit = menu.getByRole("menuitem", { name: "Edit server", exact: true });
  await expect(edit).toBeFocused();
  await expect(reinstall).toBeDisabled();
  await page.keyboard.press("ArrowDown");
  await expect(
    menu.getByRole("menuitem", { name: "Copy", exact: true }),
  ).toBeFocused();
  await prepareScreenshot(page);
  await expect(menu).toHaveScreenshot(["crops", "actions-menu.png"]);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(menu).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await expect(trigger).toBeFocused();
  await trigger.click();
  await page
    .getByRole("heading", { name: "Survival", exact: true, level: 1 })
    .click();
  await expect(menu).toBeHidden();
  await page.setViewportSize({ width: 1400, height: 1100 });
  await trigger.click();
  await kill.click();
  await expect(menu).toBeHidden();
  const danger = page.getByRole("dialog", {
    name: "Kill server: Survival",
    exact: true,
  });
  await prepareScreenshot(page);
  await expect(danger).toHaveScreenshot(["crops", "confirmation-danger.png"]);
  await danger.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Start", exact: true }),
  ).toBeEnabled();
  await trigger.click();
  await expect(kill).toBeDisabled();
  await expect(edit).toBeFocused();
  await reinstall.click();
  const dialog = page.getByRole("dialog", {
    name: "Reinstall server: Survival",
    exact: true,
  });
  const confirm = dialog.getByRole("button", {
    name: "Reinstall server",
    exact: true,
  });
  await prepareScreenshot(page);
  await expect(dialog).toHaveScreenshot([
    "crops",
    "confirmation-reinstall.png",
  ]);
  let release: () => void = () => {};
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/servers/survival/actions", async (route) => {
    await pending;
    await route.continue();
  });
  try {
    await confirm.click();
    await expect(
      dialog.getByRole("button", { name: "Close", exact: true }),
    ).toBeDisabled();
    await expect(
      dialog.getByRole("button", { name: "Cancel", exact: true }),
    ).toBeDisabled();
    await expect(
      dialog.getByRole("button", { name: "Working…", exact: true }),
    ).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
  } finally {
    release();
  }
  await expect(dialog).toHaveCount(0);
});
