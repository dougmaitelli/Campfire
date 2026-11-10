import type { Locator, Page } from "@playwright/test";
import { expect } from "./fixtures";

export async function openServerIntegrations(page: Page) {
  await page
    .getByRole("button", { name: "More server actions", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Edit server", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Edit server", exact: true });
  await drawer
    .getByRole("button", { name: "Integrations", exact: true })
    .click();
  await expect(
    drawer.getByRole("button", { name: "Add integration", exact: true }),
  ).toBeVisible();
  return drawer;
}

export async function editServerIntegration(page: Page) {
  const drawer = await openServerIntegrations(page);
  const edit = drawer
    .getByRole("button", { name: /^Edit integration for #/ })
    .first();
  if (await edit.count()) await edit.click();
  else
    await drawer
      .getByRole("button", { name: "Add integration", exact: true })
      .click();
  return drawer;
}

export async function saveServerIntegration(drawer: Locator) {
  await drawer.getByRole("button", { name: "Review", exact: true }).click();
  await drawer
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(drawer).toHaveCount(0);
}
