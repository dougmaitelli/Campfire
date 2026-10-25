import { test, expect } from "./fixtures";
import { login, openServerTab } from "./screens";

test("danger and reinstall confirmations use the right action and lock dismissal while busy", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "console", "Console");
  await page.getByRole("button", { name: "Kill", exact: true }).click();
  const danger = page.getByRole("dialog", {
    name: "Kill server: Survival",
    exact: true,
  });
  await expect(
    danger.getByRole("button", { name: "Kill server", exact: true }),
  ).toHaveClass(/button-danger/);
  await danger.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Start", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Reinstall", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Reinstall server: Survival",
    exact: true,
  });
  const confirm = dialog.getByRole("button", {
    name: "Reinstall server",
    exact: true,
  });
  await expect(confirm).toHaveClass(/button-primary/);
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
