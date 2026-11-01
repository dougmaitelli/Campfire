import { test, expect, prepareScreenshot } from "./fixtures";
import { login, openScreen } from "./screens";

test("administrators can edit OIDC names and emails through the user drawer", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "users", "Users");
  await page
    .getByRole("button", { name: "Edit user Sam Chen", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Edit user", exact: true });
  const name = drawer.getByRole("textbox", {
    name: "Name (optional)",
    exact: true,
  });
  const email = drawer.getByRole("textbox", {
    name: "Email (optional)",
    exact: true,
  });
  await expect(name).toBeEditable();
  await expect(email).toBeEditable();
  await prepareScreenshot(page);
  await expect(drawer).toHaveScreenshot("user-oidc-edit.png");
  await name.fill("Sam Updated");
  await email.fill("sam.updated@example.test");
  await drawer.getByRole("button", { name: "Save user", exact: true }).click();
  await expect(drawer).toBeHidden();
  await page.reload();
  await page
    .getByRole("button", { name: "Edit user Sam Updated", exact: true })
    .click();
  await expect(name).toHaveValue("Sam Updated");
  await expect(email).toHaveValue("sam.updated@example.test");
});
