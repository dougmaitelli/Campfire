import { test, expect, prepareScreenshot } from "./fixtures";
import { login, openServerTab } from "./screens";

const payload = (name: string, text: string) => ({
  name,
  mimeType: "text/plain",
  buffer: Buffer.from(text),
});

test("upload overwrite and rename are explicit and preserve file contents", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page
    .locator('input[type="file"]')
    .setInputFiles(payload("server.properties", "replacement"));
  const conflict = page.getByRole("dialog", {
    name: "Resolve upload conflict",
  });
  await expect(
    conflict.getByRole("radio", { name: "Skip", exact: true }),
  ).toBeChecked();
  await expect(conflict.getByRole("checkbox")).toHaveCount(0);
  expect(
    await conflict
      .getByRole("group", { name: "Action for this file" })
      .evaluate((element) => getComputedStyle(element).borderTopWidth),
  ).toBe("0px");
  await prepareScreenshot(page);
  await expect(conflict).toHaveScreenshot([
    "crops",
    "file-upload-conflict.png",
  ]);
  await page.setViewportSize({ width: 390, height: 844 });
  await prepareScreenshot(page);
  await expect(conflict).toHaveScreenshot([
    "crops",
    "file-upload-conflict-mobile.png",
  ]);
  await conflict.getByRole("radio", { name: "Overwrite", exact: true }).check();
  await conflict.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(
    page.getByText("1 file uploaded.", { exact: true }),
  ).toBeVisible();
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  await browser
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  await expect(browser.getByRole("textbox")).toHaveValue("replacement");
  await page
    .getByRole("button", { name: "Back to files", exact: true })
    .click();
  await page
    .locator('input[type="file"]')
    .setInputFiles(payload("server.properties", "renamed copy"));
  await conflict.getByRole("radio", { name: "Rename", exact: true }).check();
  await expect(
    conflict.getByRole("textbox", { name: "New filename" }),
  ).toHaveValue("server copy.properties");
  const renamedInput = conflict.getByRole("textbox", { name: "New filename" });
  await renamedInput.fill("server.properties");
  await expect(
    conflict.getByRole("button", { name: "Continue", exact: true }),
  ).toBeDisabled();
  await expect(
    conflict.getByText(
      "Choose an unused filename without slashes or control characters.",
    ),
  ).toBeVisible();
  await renamedInput.fill("server copy.properties");
  await conflict.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(conflict).toHaveCount(0);
  await browser
    .getByRole("button", { name: "server copy.properties", exact: true })
    .click();
  await expect(browser.getByRole("textbox")).toHaveValue("renamed copy");
});

test("batch duplicates require a decision and apply-to-all skips are visible", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page
    .locator('input[type="file"]')
    .setInputFiles([
      payload("repeated.txt", "first"),
      payload("repeated.txt", "second"),
      payload("server.properties", "do not replace"),
      payload("unrelated.txt", "new"),
    ]);
  const conflict = page.getByRole("dialog", {
    name: "Resolve upload conflict",
  });
  await expect(
    conflict.getByText("Another file in this upload batch uses this name.", {
      exact: true,
    }),
  ).toBeVisible();
  await conflict
    .getByRole("checkbox", { name: "Apply this action to remaining conflicts" })
    .check();
  await conflict.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(
    page.getByText(
      "Upload results: 2 completed, 2 skipped, 0 failed, 0 not uploaded.",
      { exact: true },
    ),
  ).toBeVisible();
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  await browser
    .getByRole("button", { name: "repeated.txt", exact: true })
    .click();
  await expect(browser.getByRole("textbox")).toHaveValue("first");
});

test("a destination change after conflict selection is not overwritten", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  const begins: { revision: string }[] = [];
  await page.route("**/api/v1/servers/survival/files", async (route) => {
    const input = route.request().postDataJSON() as {
      operation: string;
      path: string;
      revision: string;
    };
    if (input.operation === "upload.begin") {
      begins.push(input);
      await route.fulfill({
        status: 409,
        json: { error: "Destination changed or already exists." },
      });
    } else await route.continue();
  });
  await page
    .locator('input[type="file"]')
    .setInputFiles(payload("server.properties", "replacement"));
  const conflict = page.getByRole("dialog", {
    name: "Resolve upload conflict",
  });
  await conflict.getByRole("radio", { name: "Overwrite", exact: true }).check();
  await conflict.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(
    page.getByText(
      "Upload results: 0 completed, 0 skipped, 1 failed, 0 not uploaded.",
      { exact: true },
    ),
  ).toBeVisible();
  expect(begins).toHaveLength(1);
  expect(begins[0]!.revision).not.toBe("absent");
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  await browser
    .getByRole("button", { name: "server.properties", exact: true })
    .click();
  await expect(browser.getByRole("textbox")).toHaveValue(
    /motd=Campfire mock server/,
  );
});

test("a lost commit response is reported as unconfirmed even if publication succeeded", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  await page.route("**/api/v1/servers/survival/files", async (route) => {
    const input = route.request().postDataJSON() as { operation: string };
    if (input.operation === "upload.commit") {
      const response = await route.fetch();
      expect(response.ok()).toBe(true);
      await route.abort("failed");
    } else await route.continue();
  });
  await page
    .locator('input[type="file"]')
    .setInputFiles(payload("uncertain.txt", "published"));
  await expect(
    page.getByText(
      "Upload results: 0 completed, 0 skipped, 0 failed, 0 not uploaded, 1 unconfirmed.",
      { exact: true },
    ),
  ).toBeVisible();
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  await browser
    .getByRole("button", { name: "uncertain.txt", exact: true })
    .click();
  await expect(browser.getByRole("textbox")).toHaveValue("published");
});
