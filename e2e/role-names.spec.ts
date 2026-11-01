import { test, expect } from "./fixtures";
import { login, openScreen } from "./screens";

test("role name suggestions follow custom grants and save without being typed", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "roles", "Roles");
  await page
    .getByRole("button", { name: "+ Create role", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Create role", exact: true });
  const name = drawer.getByRole("textbox", { name: "Name", exact: true });
  await drawer
    .getByRole("checkbox", { name: "Write files", exact: true })
    .check();
  await expect(name).toHaveValue("Server · Write files");
  await drawer
    .getByRole("checkbox", { name: "Restore backups", exact: true })
    .check();
  const generated = "Server · Write files + Restore backups";
  await expect(name).toHaveValue(generated);
  const response = page.waitForResponse(
    (response) =>
      response.url().endsWith("/api/v1/roles") &&
      response.request().method() === "POST",
  );
  await drawer.getByRole("button", { name: "Save role", exact: true }).click();
  const saved = await response;
  expect(saved.ok()).toBeTruthy();
  expect(await saved.json()).toMatchObject({
    name: generated,
    scope: "server",
  });
  await page
    .getByRole("button", { name: "+ Create role", exact: true })
    .click();
  await drawer
    .getByRole("checkbox", { name: "Write files", exact: true })
    .check();
  await drawer
    .getByRole("checkbox", { name: "Restore backups", exact: true })
    .check();
  await expect(name).toHaveValue(`${generated} 2`);
  await name.fill("World maintainers");
  await drawer.getByRole("combobox", { name: "Scope", exact: true }).click();
  await page.getByRole("option", { name: "Platform", exact: true }).click();
  await expect(name).toHaveValue("World maintainers");
  await drawer.getByRole("checkbox").check();
  await expect(name).toHaveValue("World maintainers");
});

test("platform suggestions follow grants while existing role names stay unchanged", async ({
  page,
}) => {
  await login(page);
  await openScreen(page, "roles", "Roles");
  await page
    .getByRole("button", { name: "+ Create role", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Create role", exact: true });
  const name = drawer.getByRole("textbox", { name: "Name", exact: true });
  await drawer.getByRole("combobox", { name: "Scope", exact: true }).click();
  await page.getByRole("option", { name: "Platform", exact: true }).click();
  await expect(name).toHaveValue("Platform member");
  await drawer.getByRole("checkbox").check();
  await expect(name).toHaveValue("Server creator 2");
  await drawer.getByRole("button", { name: "Cancel", exact: true }).click();
  await page
    .getByRole("button", { name: "Edit role World maintainers", exact: true })
    .click();
  const editing = page.getByRole("dialog", { name: "Edit role", exact: true });
  await editing
    .getByRole("combobox", { name: "Apply a permission preset", exact: true })
    .click();
  await page.getByRole("option", { name: "Log viewer", exact: true }).click();
  await expect(
    editing.getByRole("textbox", { name: "Name", exact: true }),
  ).toHaveValue("World maintainers");
});
