import { test, expect, prepareScreenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("disabled drawer Save actions retain a muted primary tint", async ({
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
  const cancel = dialog.getByRole("button", { name: "Cancel", exact: true });
  await expect(save).toBeDisabled();
  await expect(save).toHaveClass(/button-primary/);
  const disabledColor = await save.evaluate(
    (button) => getComputedStyle(button).backgroundColor,
  );
  expect(disabledColor).not.toBe(
    await cancel.evaluate((button) => getComputedStyle(button).backgroundColor),
  );
  await prepareScreenshot(page);
  await expect(dialog.locator(".drawer-footer")).toHaveScreenshot(
    "drawer-save-disabled.png",
  );
  await page.setViewportSize({ width: 390, height: 844 });
  await prepareScreenshot(page);
  await expect(dialog.locator(".drawer-footer")).toHaveScreenshot(
    "drawer-save-disabled-mobile.png",
  );
  await dialog.getByLabel("Name", { exact: true }).fill("File readers");
  await expect(save).toBeEnabled();
  expect(
    await save.evaluate((button) => getComputedStyle(button).backgroundColor),
  ).not.toBe(disabledColor);
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
  await prepareScreenshot(page);
  await expect(dialog.locator(".drawer-footer")).toHaveScreenshot(
    "wizard-next-disabled.png",
  );
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
  await dialog.getByRole("button", { name: "Back", exact: true }).click();
  await name.fill("");
  await expect(next).toBeDisabled();
  await name.fill("Test node");
  await next.click();
  await access.click();
  await dialog
    .getByRole("button", { name: "Next: Review", exact: true })
    .click();
  await expect(
    dialog.getByRole("button", { name: "Add Spark", exact: true }),
  ).toBeEnabled();
});
