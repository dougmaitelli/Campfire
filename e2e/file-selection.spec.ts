import type { Locator, Page } from "@playwright/test";
import { test, expect } from "./fixtures";
import { login, openServerTab } from "./screens";
import { FileKind, FileOperation } from "../web/src/contracts/domains";
import type { FileRequest } from "../web/src/api/fileTypes";

const names = [
  "a-selection.txt",
  "b-selection.txt",
  "c-selection.txt",
] as const;
async function prepare(page: Page) {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page.locator('input[type="file"]').setInputFiles(
    names.map((name) => ({
      name,
      mimeType: "text/plain",
      buffer: Buffer.from(name),
    })),
  );
  for (const name of names)
    await expect(page.getByRole("button", { name, exact: true })).toBeVisible();
  return page.getByRole("region", { name: "File browser", exact: true });
}
async function dragFile(page: Page, name: string, target: Locator) {
  const source = page.getByRole("button", { name, exact: true });
  await source.scrollIntoViewIfNeeded();
  const start = (await source.boundingBox())!;
  await page.mouse.move(start.x + start.width / 2, start.y + start.height / 2);
  await page.mouse.down();
  // Start the native drag before scrolling to an off-screen target. dragTo's
  // pre-drag target scrolling otherwise moves the source out from under the mouse.
  await page.mouse.move(
    start.x + start.width / 2 + 12,
    start.y + start.height / 2,
    { steps: 3 },
  );
  await expect(page.locator(".file-drag-status")).toBeVisible();
  await target.scrollIntoViewIfNeeded();
  const end = (await target.boundingBox())!;
  await page.mouse.move(end.x + end.width / 2, end.y + end.height / 2);
  await page.mouse.move(end.x + end.width / 2, end.y + end.height / 2);
  await page.mouse.up();
}

async function selectGroup(page: Page) {
  await page
    .getByRole("checkbox", { name: `Select ${names[0]}`, exact: true })
    .check();
  await page
    .getByRole("checkbox", { name: `Select ${names[2]}`, exact: true })
    .click({ modifiers: ["Shift"] });
  for (const name of names)
    await expect(
      page.getByRole("checkbox", { name: `Select ${name}`, exact: true }),
    ).toBeChecked();
}

test("file selection supports range, additive, keyboard and ordinary filename activation", async ({
  page,
}) => {
  const browser = await prepare(page);
  await selectGroup(page);
  const header = browser.getByRole("checkbox", {
    name: "Select all items",
    exact: true,
  });
  await expect(header).toHaveJSProperty("indeterminate", true);
  expect(
    await header.evaluate(
      (input) => getComputedStyle(input, "::before").height,
    ),
  ).toBe("2px");
  await browser
    .getByRole("button", { name: names[1], exact: true })
    .click({ modifiers: ["ControlOrMeta"] });
  await expect(
    browser.getByRole("checkbox", { name: `Select ${names[1]}`, exact: true }),
  ).not.toBeChecked();
  const row = browser.locator(`tr[data-selection-id="${names[0]}"]`);
  await row.focus();
  await page.keyboard.press("ControlOrMeta+a");
  await expect(
    browser.getByRole("checkbox", { name: "Select all items", exact: true }),
  ).toBeChecked();
  await page.keyboard.press("Escape");
  await expect(
    browser.locator('tbody input[type="checkbox"]:checked'),
  ).toHaveCount(0);
  await row.focus();
  await page.keyboard.type("b-selection");
  await expect(
    browser.getByRole("checkbox", { name: `Select ${names[1]}`, exact: true }),
  ).toBeChecked();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("textbox", { name: `/${names[1]}`, exact: true }),
  ).toBeVisible();
});

test("mouse marquee selects rows without opening them and Escape restores the starting selection", async ({
  page,
}, testInfo) => {
  const browser = await prepare(page);
  const first = browser.locator(
    `tr[data-selection-id="${names[0]}"] td.table-date`,
  );
  const last = browser.locator(
    `tr[data-selection-id="${names[2]}"] td.table-date`,
  );
  await first.scrollIntoViewIfNeeded();
  const a = (await first.boundingBox())!;
  const c = (await last.boundingBox())!;
  await page.mouse.move(a.x + a.width / 2, a.y + 2);
  await page.mouse.down();
  await page.mouse.move(c.x + c.width / 2 + 12, c.y + c.height - 2, {
    steps: 12,
  });
  await expect(page.locator(".table-selection-marquee")).toBeVisible();
  for (const name of names)
    await expect(
      browser.getByRole("checkbox", { name: `Select ${name}`, exact: true }),
    ).toBeChecked();
  await page.mouse.up();
  await expect(page.locator(".table-selection-marquee")).toHaveCount(0);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await browser.screenshot({ path: testInfo.outputPath("file-selection.png") });
  await browser
    .getByRole("button", { name: "Clear selection", exact: true })
    .click();
  await page.mouse.move(a.x + a.width / 2, a.y + 2);
  await page.mouse.down();
  await page.mouse.move(c.x + c.width / 2 + 12, c.y + c.height - 2, {
    steps: 12,
  });
  await page.keyboard.press("Escape");
  await page.mouse.up();
  await expect(
    browser.locator('tbody input[type="checkbox"]:checked'),
  ).toHaveCount(0);
});

test("native group drag reviews the target, moves files, and supports breadcrumb destinations without uploading", async ({
  page,
}) => {
  const browser = await prepare(page);
  let uploads = 0;
  page.on("request", (request) => {
    if (
      request.url().endsWith("/survival/files") &&
      (request.postDataJSON() as FileRequest).operation ===
        FileOperation.UploadBegin
    )
      uploads++;
  });
  await selectGroup(page);
  await dragFile(
    page,
    names[0],
    browser.locator('tr[data-selection-id="world"]'),
  );
  const dialog = page.getByRole("dialog", {
    name: "Move selected items",
    exact: true,
  });
  await expect(
    dialog.getByRole("textbox", { name: "Destination folder", exact: true }),
  ).toHaveValue("/world/");
  await expect(dialog).toContainText("3 selected items");
  await dialog.getByRole("button", { name: "Move", exact: true }).click();
  await expect(dialog).toBeHidden();
  for (const name of names)
    await expect(
      browser.getByRole("button", { name, exact: true }),
    ).toHaveCount(0);
  await browser.getByRole("button", { name: "world", exact: true }).click();
  for (const name of names)
    await expect(
      browser.getByRole("button", { name, exact: true }),
    ).toBeVisible();
  await dragFile(
    page,
    names[0],
    browser.getByRole("button", { name: "Server data", exact: true }),
  );
  await expect(
    dialog.getByRole("textbox", { name: "Destination folder", exact: true }),
  ).toHaveValue("/");
  await dialog.getByRole("button", { name: "Move", exact: true }).click();
  await expect(dialog).toBeHidden();
  await browser
    .getByRole("button", { name: "Server data", exact: true })
    .click();
  await expect(
    browser.getByRole("button", { name: names[0], exact: true }),
  ).toBeVisible();
  expect(uploads).toBe(0);
});

test("drop conflicts use the same no-overwrite move dialog and can be cancelled", async ({
  page,
}) => {
  const browser = await prepare(page);
  let renames = 0;
  await page.route("**/api/v1/servers/survival/files", async (route) => {
    const input = route.request().postDataJSON() as FileRequest;
    if (input.operation === FileOperation.List && input.path === "/world/") {
      await route.fulfill({
        json: {
          server_id: "survival",
          path: "/world/",
          entries: [
            {
              name: names[0],
              kind: FileKind.File,
              size: 10,
              modified_at: "2026-11-01T00:00:00Z",
            },
          ],
        },
      });
    } else {
      if (input.operation === FileOperation.Rename) renames++;
      await route.continue();
    }
  });
  await dragFile(
    page,
    names[0],
    browser.locator('tr[data-selection-id="world"]'),
  );
  const dialog = page.getByRole("dialog", {
    name: "Move selected items",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Move", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText(
    "An item with the same name already exists",
  );
  expect(renames).toBe(0);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    browser.getByRole("button", { name: names[0], exact: true }),
  ).toBeVisible();
});

test("checkboxes and action buttons inside selected rows do not initiate native moves", async ({
  page,
}) => {
  const browser = await prepare(page);
  for (const control of [
    browser.getByRole("checkbox", { name: `Select ${names[0]}`, exact: true }),
    browser.getByRole("button", { name: `Download ${names[0]}`, exact: true }),
  ]) {
    const clear = browser.getByRole("button", {
      name: "Clear selection",
      exact: true,
    });
    if (await clear.isEnabled()) await clear.click();
    await selectGroup(page);
    await control.scrollIntoViewIfNeeded();
    const start = (await control.boundingBox())!;
    const end = (await browser
      .locator('tr[data-selection-id="config"]')
      .boundingBox())!;
    await page.mouse.move(
      start.x + start.width / 2,
      start.y + start.height / 2,
    );
    await page.mouse.down();
    await page.mouse.move(
      start.x + start.width / 2 + 12,
      start.y + start.height / 2,
      { steps: 3 },
    );
    await page.mouse.move(end.x + end.width / 2, end.y + end.height / 2, {
      steps: 5,
    });
    await page.mouse.move(end.x + end.width / 2, end.y + end.height / 2);
    await page.mouse.up();
    await expect(page.locator(".file-drag-status")).toHaveCount(0);
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(
      browser.getByRole("button", { name: names[0], exact: true }),
    ).toBeVisible();
  }
});

test("a folder cannot be dragged into itself", async ({ page }) => {
  const browser = await prepare(page);
  await dragFile(
    page,
    "world",
    browser.locator('tr[data-selection-id="world"]'),
  );
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(
    browser.getByRole("checkbox", { name: "Select world", exact: true }),
  ).toBeChecked();
  await expect(
    browser.getByRole("button", { name: "server.properties", exact: true }),
  ).toBeVisible();
});

test("OS files dropped on a folder row upload to the open folder, not a move target", async ({
  page,
}) => {
  const browser = await prepare(page);
  const transfer = await page.evaluateHandle(() => {
    const data = new DataTransfer();
    data.items.add(new File(["external file"], "from-os.txt"));
    return data;
  });
  try {
    const row = browser.locator('tr[data-selection-id="world"]');
    await row.dispatchEvent("dragenter", { dataTransfer: transfer });
    await expect(
      page.getByText("Files will be uploaded to /", { exact: true }),
    ).toBeVisible();
    await row.dispatchEvent("drop", { dataTransfer: transfer });
    await expect(
      browser.getByRole("button", { name: "from-os.txt", exact: true }),
    ).toBeVisible();
    await expect(page.getByRole("dialog")).toHaveCount(0);
  } finally {
    await transfer.dispose();
  }
});

test("partial moves stop, report confirmed progress and retain only remaining selections", async ({
  page,
}) => {
  const browser = await prepare(page);
  const attempted: string[] = [];
  await page.route("**/api/v1/servers/survival/files", async (route) => {
    const input = route.request().postDataJSON() as FileRequest;
    if (input.operation === FileOperation.Rename) {
      attempted.push(input.path);
      if (input.path.endsWith(names[1]!)) {
        await route.fulfill({
          status: 409,
          json: { error: "Destination changed." },
        });
        return;
      }
    }
    await route.continue();
  });
  await selectGroup(page);
  await browser.getByRole("button", { name: "Move", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Move selected items",
    exact: true,
  });
  await dialog
    .getByRole("textbox", { name: "Destination folder", exact: true })
    .fill("/world/");
  await dialog.getByRole("button", { name: "Move", exact: true }).click();
  await expect(dialog.getByRole("alert")).toContainText(
    "1 of 3 moves confirmed",
  );
  expect(attempted).toEqual(names.slice(0, 2).map((name) => `/${name}`));
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    browser.getByRole("checkbox", { name: `Select ${names[0]}`, exact: true }),
  ).toHaveCount(0);
  for (const name of names.slice(1))
    await expect(
      browser.getByRole("checkbox", { name: `Select ${name}`, exact: true }),
    ).toBeChecked();
});

test("marquee edge scrolling reaches rows beyond the initial viewport", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/v1/servers/survival/files", (route) =>
    route.fulfill({
      json: {
        server_id: "survival",
        path: "/",
        entries: Array.from({ length: 60 }, (_, index) => ({
          name: `entry-${String(index).padStart(2, "0")}.txt`,
          kind: FileKind.File,
          size: 12,
          modified_at: "2026-11-01T00:00:00Z",
        })),
      },
    }),
  );
  await page.goto("/#servers/survival/files");
  const first = page.locator(
    'tr[data-selection-id="entry-00.txt"] td.table-date',
  );
  await first.scrollIntoViewIfNeeded();
  const box = (await first.boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + 2);
  await page.mouse.down();
  await page.mouse.move(
    box.x + box.width / 2 + 10,
    page.viewportSize()!.height - 3,
    { steps: 12 },
  );
  await expect(
    page.getByRole("checkbox", { name: "Select entry-20.txt", exact: true }),
  ).toBeChecked();
  await page.mouse.up();
  await expect(page.locator(".table-selection-marquee")).toHaveCount(0);
});

test.describe("touch selection", () => {
  test.use({
    isMobile: true,
    hasTouch: true,
  });
  test("checkboxes and explicit Move remain available without drag gestures", async ({
    page,
  }, testInfo) => {
    const browser = await prepare(page);
    await page.setViewportSize({ width: 390, height: 844 });
    await browser
      .getByRole("checkbox", { name: `Select ${names[0]}`, exact: true })
      .tap();
    await expect(
      browser.getByRole("checkbox", {
        name: `Select ${names[0]}`,
        exact: true,
      }),
    ).toBeChecked();
    await browser.getByRole("button", { name: "Move", exact: true }).tap();
    const dialog = page.getByRole("dialog", {
      name: "Move selected items",
      exact: true,
    });
    await dialog
      .getByRole("textbox", { name: "Destination folder", exact: true })
      .fill("/world/");
    await dialog.screenshot({
      path: testInfo.outputPath("file-move-mobile.png"),
    });
    await dialog.getByRole("button", { name: "Move", exact: true }).tap();
    await expect(dialog).toBeHidden();
    await expect(page.locator(".table-selection-marquee")).toHaveCount(0);
    await expect(
      browser.getByRole("button", { name: names[0], exact: true }),
    ).toHaveCount(0);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  });
});
