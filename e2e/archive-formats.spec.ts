import { test, expect } from "./fixtures";
import { login, openServerTab } from "./screens";

test("additional tar formats expose extraction with the complete suffix removed", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  const extensions = [
    "tar",
    "tar.xz",
    "tar.lz",
    "tar.bz2",
    "tar.lz4",
    "tar.zst",
    "tar.zstd",
    "TXZ",
    "tlz",
    "tbz2",
    "tzst",
  ];
  await page.locator('input[type="file"]').setInputFiles([
    ...extensions.map((extension) => ({
      name: `world.${extension}`,
      mimeType: "application/octet-stream",
      buffer: Buffer.from("fixture"),
    })),
    {
      name: "unsupported.7z",
      mimeType: "application/octet-stream",
      buffer: Buffer.from("fixture"),
    },
  ]);
  for (const extension of extensions) {
    await page
      .getByRole("button", { name: `Extract world.${extension}`, exact: true })
      .click();
    const dialog = page.getByRole("dialog", {
      name: "Extract archive",
      exact: true,
    });
    await expect(
      dialog.getByRole("textbox", { name: "Destination folder", exact: true }),
    ).toHaveValue("world");
    await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  }
  await expect(
    page.getByRole("button", { name: "Extract unsupported.7z", exact: true }),
  ).toHaveCount(0);
});

test("ZIP names use the observable compression flow", async ({ page }) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page.locator('input[type="file"]').setInputFiles({
    name: "zip-source.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("saved world"),
  });
  await page
    .getByRole("checkbox", { name: "Select zip-source.txt", exact: true })
    .check();
  await page.getByRole("button", { name: "Archive", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Archive", exact: true });
  await expect(dialog).toContainText("End the name in .tar.gz, .tgz, or .zip.");
  await dialog
    .getByRole("textbox", { name: "Archive name", exact: true })
    .fill("selected.zip");
  let submitted = false;
  await page.route("**/api/v1/servers/survival/files", async (route) => {
    const request = route.request().postDataJSON();
    if (request.operation !== "compress.start") return route.continue();
    expect(request.destination).toBe("/selected.zip");
    expect(request.paths).toEqual(["/zip-source.txt"]);
    submitted = true;
    await route.fulfill({
      json: {
        server_id: "survival",
        path: "/selected.zip",
        compression: {
          id: request.compression.id,
          state: "completed",
          expires_at: new Date(Date.now() + 900000).toISOString(),
        },
      },
    });
  });
  await dialog.getByRole("button", { name: "Archive", exact: true }).click();
  await expect(dialog).toBeHidden();
  expect(submitted).toBe(true);
});
