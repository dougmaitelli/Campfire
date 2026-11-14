import { test, expect, screenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("drawer Save actions disable until required fields are provided", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "roles", "Roles");
  await page
    .getByRole("button", { name: "+ Create role", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "Create role", exact: true });
  await expect(dialog.getByRole("checkbox")).toHaveCount(18);
  const save = dialog.getByRole("button", { name: "Save role", exact: true });
  await expect(save).toBeEnabled();
  await dialog.getByLabel("Name", { exact: true }).fill("");
  await expect(save).toBeDisabled();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(save).toBeDisabled();
  await dialog.getByLabel("Name", { exact: true }).fill("File readers");
  await expect(save).toBeEnabled();
  await page.setViewportSize({ width: 1400, height: 900 });
  await expect(save).toBeEnabled();
});

test("wizard Next disables immediately for missing and invalid required fields", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "sparks", "Sparks");
  await page.getByRole("button", { name: "+ Add Spark", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add Spark", exact: true });
  const name = dialog.getByLabel("Name", { exact: true });
  const next = dialog.getByRole("button", {
    name: "Next: Resources & networking",
    exact: true,
  });
  await expect(next).toBeDisabled();
  await name.fill("   ");
  await expect(next).toBeDisabled();
  await name.fill("Test node");
  await expect(next).toBeEnabled();
  await next.click();
  const access = dialog.getByRole("button", {
    name: "Next: Access",
    exact: true,
  });
  const cpu = dialog.getByLabel("CPU cores", { exact: true });
  await expect(access).toBeEnabled();
  await cpu.fill("");
  await expect(access).toBeDisabled();
  await cpu.fill("-1");
  await expect(access).toBeDisabled();
  await cpu.fill("4");
  await expect(access).toBeEnabled();
  await dialog
    .getByRole("button", { name: "Add port range", exact: true })
    .click();
  await screenshot(page, "spark-resources-networking.png", dialog);
  await dialog.getByRole("button", { name: "Back", exact: true }).click();
  await name.fill("");
  await expect(next).toBeDisabled();
  await name.fill("Test node");
  await next.click();
  await access.click();
  await screenshot(page, "spark-access.png", dialog);
  await dialog
    .getByRole("button", { name: "Next: Review", exact: true })
    .click();
  await expect(
    dialog.getByRole("button", { name: "Add Spark", exact: true }),
  ).toBeEnabled();
  await screenshot(page, "spark-review.png", dialog);
});

test("drawer actions support keyboard navigation and dismiss before confirmations", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "sparks", "Sparks");
  await page
    .getByRole("button", { name: "View details for Europe", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: /^Europe/ });
  await expect(
    drawer.getByText("Primary game server node", { exact: true }),
  ).toBeVisible();
  await screenshot(page, "spark-inspection.png", drawer);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "spark-inspection-mobile.png", drawer);
  await page.setViewportSize({ width: 1400, height: 900 });
  const trigger = drawer.getByRole("button", { name: "Actions", exact: true });
  const menu = drawer.getByRole("menu", { name: "Actions", exact: true });
  const reenroll = menu.getByRole("menuitem", {
    name: "Re-enroll",
    exact: true,
  });
  await trigger.click();
  await expect(menu).toBeVisible();
  await expect(
    menu.getByRole("menuitem", { disabled: false }).first(),
  ).toBeFocused();
  await page.keyboard.press("End");
  await expect(reenroll).toBeFocused();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(menu).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(menu).toBeHidden();
  await expect(drawer).toBeVisible();
  await expect(trigger).toBeFocused();
  await trigger.click();
  await drawer.getByRole("heading", { level: 2 }).click();
  await expect(menu).toBeHidden();
  await trigger.click();
  await reenroll.click();
  const confirmation = page.getByRole("dialog", {
    name: "Re-enroll Europe?",
    exact: true,
  });
  await expect(confirmation).toBeVisible();
  await confirmation
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await expect(drawer).toBeVisible();
  await expect(menu).toBeHidden();
  await drawer
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await expect(drawer).toHaveCount(0);
});

test("server page actions open the transfer drawer", async ({ page }) => {
  await login(page);
  await openScreen(page, "servers", "Servers");
  await page
    .getByRole("button", { name: "Open Survival", exact: true })
    .click();
  await page
    .getByRole("button", { name: "More server actions", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Transfer", exact: true }).click();
  const transfer = page.getByRole("dialog", { name: /^Transfer/ });
  await expect(transfer).toBeVisible();
  await expect(
    transfer.getByRole("heading", { name: "Transfer history", exact: true }),
  ).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(1);
});
