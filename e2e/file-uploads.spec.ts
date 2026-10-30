import type { Page } from "@playwright/test";
import { test, expect, screenshot } from "./fixtures";
import { login, openServerTab } from "./screens";

async function draggedFiles(
  page: Page,
  files: { name: string; size: number }[],
) {
  return page.evaluateHandle((entries) => {
    const transfer = new DataTransfer();
    for (const entry of entries) {
      transfer.items.add(new File([new Uint8Array(entry.size)], entry.name));
    }
    return transfer;
  }, files);
}

test("file drops show a stable overlay and upload into the open folder", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  const overlay = page.getByText("Release to upload", { exact: true });
  const transfer = await draggedFiles(page, [
    { name: "dropped.txt", size: 12 },
    { name: "empty.txt", size: 0 },
  ]);
  const text = await page.evaluateHandle(() => {
    const data = new DataTransfer();
    data.setData("text/plain", "ordinary text");
    return data;
  });
  await browser.dispatchEvent("dragenter", { dataTransfer: text });
  await expect(overlay).toHaveCount(0);
  await browser.dispatchEvent("dragenter", { dataTransfer: transfer });
  const row = browser.getByRole("button", {
    name: "server.properties",
    exact: true,
  });
  await row.dispatchEvent("dragenter", { dataTransfer: transfer });
  await row.dispatchEvent("dragleave", { dataTransfer: transfer });
  await expect(overlay).toBeVisible();
  await screenshot(page, "file-upload-drop.png");
  await page.setViewportSize({ width: 390, height: 844 });
  await browser.scrollIntoViewIfNeeded();
  await screenshot(page, "file-upload-drop-mobile.png");
  await browser.dispatchEvent("dragleave", { dataTransfer: transfer });
  await expect(overlay).toHaveCount(0);
  expect(
    await page.evaluate((data) => {
      const event = new DragEvent("drop", {
        dataTransfer: data,
        bubbles: true,
        cancelable: true,
      });
      document.body.dispatchEvent(event);
      return event.defaultPrevented;
    }, transfer),
  ).toBe(true);
  await expect(
    browser.getByRole("button", { name: "dropped.txt", exact: true }),
  ).toHaveCount(0);
  await page.setViewportSize({ width: 1400, height: 1100 });
  await browser.getByRole("button", { name: "world", exact: true }).click();
  await expect(
    page.getByText("Files will be uploaded to /world/", { exact: true }),
  ).toHaveCount(0);
  await browser.dispatchEvent("dragenter", { dataTransfer: transfer });
  await expect(
    page.getByText("Files will be uploaded to /world/", { exact: true }),
  ).toBeVisible();
  await browser.dispatchEvent("drop", { dataTransfer: transfer });
  await expect(
    page.getByText("2 files uploaded.", { exact: true }),
  ).toBeVisible();
  for (const name of ["dropped.txt", "empty.txt"]) {
    await expect(
      browser.getByRole("button", { name, exact: true }),
    ).toBeVisible();
  }
  await page.locator('input[type="file"]').setInputFiles({
    name: "picker.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("picker contents"),
  });
  await expect(
    browser.getByRole("button", { name: "picker.txt", exact: true }),
  ).toBeVisible();
  await browser
    .getByRole("button", { name: "picker.txt", exact: true })
    .click();
  await expect(browser.getByRole("textbox")).toHaveValue("picker contents");
  await page
    .getByRole("button", { name: "Back to files", exact: true })
    .click();
  await page.locator('input[type="file"]').setInputFiles({
    name: "picker.txt",
    mimeType: "text/plain",
    buffer: Buffer.from("overwrite"),
  });
  await expect(page.getByRole("alert")).toContainText(
    "Destination already exists.",
  );
});

test("upload progress measures the queue and cancellation keeps completed files", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  const operations: { operation: string; path: string; offset?: number }[] = [];
  let release = () => {};
  const held = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/v1/servers/survival/files", async (route) => {
    const input = route.request().postDataJSON() as {
      operation: string;
      path: string;
      offset?: number;
    };
    operations.push(input);
    if (input.operation === "upload.chunk" && input.offset === 1024 ** 2) {
      await held;
      await route.abort();
    } else {
      await route.continue();
    }
  });
  const transfer = await draggedFiles(page, [
    { name: "completed.txt", size: 6 },
    {
      name: "long-world-backup-filename-that-needs-to-fit-on-a-small-screen.dat",
      size: 3 * 1024 ** 2,
    },
    { name: "remaining.txt", size: 6 },
  ]);
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  try {
    await browser.dispatchEvent("drop", { dataTransfer: transfer });
    const progress = page.getByRole("progressbar", {
      name: "Upload progress",
      exact: true,
    });
    await expect(progress).toHaveAttribute("value", "33");
    await expect(
      page.getByText("Uploading file 2 of 3", { exact: true }),
    ).toBeVisible();
    await expect(progress).toHaveAttribute("aria-valuetext", "1 MiB of 3 MiB");
    await screenshot(page, "file-upload-progress.png");
    await page.setViewportSize({ width: 390, height: 844 });
    await page
      .getByRole("region", { name: "Upload progress", exact: true })
      .scrollIntoViewIfNeeded();
    await screenshot(page, "file-upload-progress-mobile.png");
    await page
      .getByRole("button", { name: "Cancel upload", exact: true })
      .click();
    release();
    await expect(
      page.getByText("Upload cancelled. Completed files are kept.", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(progress).toHaveCount(0);
    await expect(
      browser.getByRole("button", { name: "completed.txt", exact: true }),
    ).toBeVisible();
    await expect(
      browser.getByRole("button", { name: "remaining.txt", exact: true }),
    ).toHaveCount(0);
    expect(
      operations
        .filter((item) => item.operation === "upload.begin")
        .map((item) => item.path),
    ).toEqual([
      "/completed.txt",
      "/long-world-backup-filename-that-needs-to-fit-on-a-small-screen.dat",
    ]);
    expect(
      operations.filter((item) => item.operation === "upload.commit"),
    ).toHaveLength(1);
    expect(
      operations.filter((item) => item.operation === "upload.cancel"),
    ).toHaveLength(1);
  } finally {
    release();
  }
});

test("folder drops and oversized files are rejected before an upload starts", async ({
  page,
}) => {
  await login(page);
  await page.route("**/files/limits", (route) =>
    route.fulfill({ json: { max_upload_bytes: 1024 ** 2 } }),
  );
  await openServerTab(page, "files", "Files");
  const operations: string[] = [];
  page.on("request", (request) => {
    if (
      request.method() === "POST" &&
      request.url().endsWith("/survival/files")
    ) {
      operations.push(
        (request.postDataJSON() as { operation: string }).operation,
      );
    }
  });
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  const folder = await page.evaluateHandle(() => {
    const transfer = new DataTransfer();
    transfer.items.add(new File([], "folder"));
    Object.defineProperty(transfer, "items", {
      value: [{ webkitGetAsEntry: () => ({ isDirectory: true }) }],
    });
    return transfer;
  });
  await browser.dispatchEvent("drop", { dataTransfer: folder });
  await expect(page.getByRole("alert")).toContainText(
    "Drop files only. Folder uploads are not supported.",
  );
  const files = await draggedFiles(page, [
    { name: "small.txt", size: 1 },
    { name: "too-large.txt", size: 1024 ** 2 + 1 },
  ]);
  await browser.dispatchEvent("drop", { dataTransfer: files });
  await expect(page.getByRole("alert")).toContainText(
    "Uploads are limited to 1 MiB per file.",
  );
  expect(operations).not.toContain("upload.begin");
});
