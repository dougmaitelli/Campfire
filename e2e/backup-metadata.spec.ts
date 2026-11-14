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
  }
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
  await page.getByRole("searchbox").fill("Before game update");
  row = page.getByRole("row").filter({ hasText: archive });
  await expect(row).toHaveCount(1);
  await expect(
    row.getByRole("img", { name: "Pinned", exact: true }),
  ).toBeVisible();
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
