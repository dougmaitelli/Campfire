import { test, expect } from "./fixtures";
import { login, openServerTab } from "./screens";

test("RAR extraction shows progress and allows retry after a failure", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page.locator('input[type="file"]').setInputFiles({
    name: "test-archive.RAR",
    mimeType: "application/vnd.rar",
    buffer: Buffer.from("archive contents"),
  });
  await page
    .getByRole("button", { name: "Extract test-archive.RAR", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Extract archive",
    exact: true,
  });
  const input = dialog.getByRole("textbox", {
    name: "New folder name",
    exact: true,
  });
  await expect(input).toHaveValue("test-archive");
  let release = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  let attempts = 0;
  let refreshed = false;
  await page.route("**/api/v1/servers/survival/files", async (route) => {
    const request = route.request().postDataJSON() as {
      operation: string;
      path: string;
      destination?: string;
    };
    if (request.operation === "extract") {
      expect(request.path).toBe("/test-archive.RAR");
      expect(request.destination).toBe("/test-archive");
      attempts++;
      if (attempts === 1) {
        await held;
        await route.fulfill({
          status: 400,
          json: { error: "Invalid archive checksum." },
        });
      } else {
        await route.fulfill({
          json: { server_id: "survival", path: "/test-archive/" },
        });
      }
    } else {
      if (request.operation === "list") refreshed = true;
      await route.continue();
    }
  });
  try {
    await dialog.getByRole("button", { name: "Extract", exact: true }).click();
    const progress = dialog.getByRole("progressbar", {
      name: "Extracting test-archive.RAR…",
      exact: true,
    });
    await expect(progress).toBeVisible();
    await expect(progress).not.toHaveAttribute("aria-valuenow");
    await expect(input).toBeDisabled();
    await expect(
      dialog.getByRole("button", { name: "Cancel", exact: true }),
    ).toBeDisabled();
    await expect(
      dialog.getByRole("button", { name: "Close", exact: true }),
    ).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(dialog).toBeVisible();
    await page.evaluate(() => document.fonts.ready);
    await page.mouse.move(0, 0);
    // This screenshot intentionally captures an operation with aria-busy controls.
    await expect(page).toHaveScreenshot("file-extract-progress.png");
    await page.setViewportSize({ width: 390, height: 844 });
    await expect(page).toHaveScreenshot("file-extract-progress-mobile.png");
    release();
    await expect(progress).toHaveCount(0);
    await expect(dialog.getByRole("alert")).toHaveText(
      "Invalid archive checksum.",
    );
    await expect(input).toBeEnabled();
    await dialog.getByRole("button", { name: "Extract", exact: true }).click();
    await expect(dialog).toHaveCount(0);
    expect(attempts).toBe(2);
    expect(refreshed).toBe(true);
  } finally {
    release();
  }
});
