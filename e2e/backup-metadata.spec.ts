import { test, expect } from "./fixtures";
import { login, openScreen, openServerTab } from "./screens";

test("backup names and pins persist across server and catalog views", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "backups", "Backups");
  const archive = "mock-before-update.tar.gz";
  let row = page.getByRole("row").filter({ hasText: archive });
  await expect(row.getByRole("button").last()).toHaveAttribute(
    "aria-haspopup",
    "menu",
  );
  for (const name of ["Restore", "Delete local"]) {
    const action = row.getByRole("button", { name, exact: true });
    await expect(action).toHaveAttribute("title", name);
    await expect(action).toHaveText("");
  }
  for (const width of [1400, 390]) {
    await page.setViewportSize({ width, height: 900 });
    const table = await page
      .getByRole("table", { name: "Server backups", exact: true })
      .boundingBox();
    for (const name of ["Create backup", "Upload backup"]) {
      const action = await page
        .getByRole("button", { name, exact: true })
        .boundingBox();
      expect(action!.y + action!.height).toBeLessThan(table!.y);
    }
  }
  await page.setViewportSize({ width: 1400, height: 900 });
  await row
    .getByRole("button", { name: `Backup actions for ${archive}`, exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Rename backup", exact: true })
    .click();
  let dialog = page.getByRole("dialog", { name: "Rename backup", exact: true });
  const field = dialog.getByRole("textbox", {
    name: "Backup name",
    exact: true,
  });
  await expect(dialog.locator("label")).toHaveCSS("text-align", "start");
  const labelBounds = await dialog.locator("label").boundingBox();
  const inputBounds = await field.boundingBox();
  expect(labelBounds!.y + labelBounds!.height).toBeLessThan(inputBounds!.y);
  await field.fill("Before game update");
  await field.press("Enter");
  await expect(dialog).toHaveCount(0);
  await expect(row).toContainText("Before game update");
  await row
    .getByRole("button", {
      name: "Backup actions for Before game update",
      exact: true,
    })
    .click();
  await page.getByRole("menuitem", { name: "Pin backup", exact: true }).click();
  await expect(
    row.getByRole("img", { name: "Pinned", exact: true }),
  ).toBeVisible();
  await expect(
    row.getByRole("button", { name: "Delete local", exact: true }),
  ).toBeDisabled();
  await expect(
    row.getByRole("link", { name: "Download Before game update", exact: true }),
  ).toHaveAttribute("href", `/api/v1/servers/survival/backups/${archive}`);

  await page.reload();
  await openScreen(page, "backups", "Backups");
  await page.setViewportSize({ width: 390, height: 844 });
  const catalog = page.getByRole("table", { name: "All server backups" });
  const catalogBounds = await catalog.boundingBox();
  const containerBounds = await catalog.locator("..").boundingBox();
  expect(catalogBounds!.x + catalogBounds!.width).toBeLessThanOrEqual(
    containerBounds!.x + containerBounds!.width,
  );
  await page.getByRole("searchbox").fill("Before game update");
  row = page.getByRole("row").filter({ hasText: archive });
  await expect(row).toHaveCount(1);
  await expect(
    row.getByRole("img", { name: "Pinned", exact: true }),
  ).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  const archiveCell = row.locator("td").first();
  await expect(archiveCell).toHaveCSS("min-width", "96px");
  expect((await archiveCell.boundingBox())!.width).toBeGreaterThanOrEqual(96);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBeTruthy();
  await row
    .getByRole("button", {
      name: "Backup actions for Before game update",
      exact: true,
    })
    .click();
  await page
    .getByRole("menuitem", { name: "Rename backup", exact: true })
    .click();
  dialog = page.getByRole("dialog", { name: "Rename backup", exact: true });
  await dialog.getByText("Backup name", { exact: true }).click();
  await expect(dialog.getByRole("textbox")).toBeFocused();
  await expect(page).toHaveURL(/#backups$/);
  await expect(dialog.locator("label")).toHaveCSS("text-align", "start");
  await dialog.getByRole("textbox").fill("Before major update");
  await dialog
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await page.getByRole("searchbox").fill("Before major update");
  await row
    .getByRole("button", {
      name: "Backup actions for Before major update",
      exact: true,
    })
    .click();
  await page
    .getByRole("menuitem", { name: "Unpin backup", exact: true })
    .click();
  await expect(
    row.getByRole("img", { name: "Pinned", exact: true }),
  ).toHaveCount(0);
  await page.reload();
  await openServerTab(page, "backups", "Backups");
  row = page.getByRole("row").filter({ hasText: archive });
  await expect(row).toContainText("Before major update");
  await expect(
    row.getByRole("button", { name: "Delete local", exact: true }),
  ).toBeEnabled();
  const audit = await page.request.get("/api/v1/audit");
  expect(audit.ok()).toBeTruthy();
  expect(await audit.json()).toEqual(
    expect.arrayContaining(
      ["rename", "pin", "unpin"].map((change) =>
        expect.objectContaining({
          action: "server.backup.update",
          target: `server:survival backup:${archive} changes:${change}`,
          outcome: "succeeded",
        }),
      ),
    ),
  );
});
