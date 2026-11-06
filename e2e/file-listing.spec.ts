import { test, expect } from "./fixtures";
import { login, openServerTab } from "./screens";

test("failed file listings use the table error state and retry to a real empty folder", async ({
  page,
}) => {
  await login(page);
  let fail = true;
  await page.route("**/api/v1/servers/survival/files", (route) =>
    route.fulfill(
      fail
        ? { status: 403, json: { error: "File access is unavailable." } }
        : { json: { server_id: "survival", path: "/", entries: [] } },
    ),
  );
  await page.goto("/#servers/survival/files");
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  const empty = browser.getByText("This folder is empty.", { exact: true });
  await expect(
    browser.getByRole("heading", { name: "Unable to load Files" }),
  ).toBeVisible();
  await expect(browser.getByRole("alert")).toContainText(
    "File access is unavailable.",
  );
  await expect(empty).toHaveCount(0);
  fail = false;
  await browser.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(empty).toBeVisible();
  await expect(browser.getByRole("alert")).toHaveCount(0);
});

test("failed navigation preserves listed files and offers a retry without a stale error", async ({
  page,
}) => {
  await login(page);
  await openServerTab(page, "files", "Files");
  const browser = page.getByRole("region", {
    name: "File browser",
    exact: true,
  });
  await page.route("**/api/v1/servers/survival/files", (route) =>
    route.fulfill({
      status: 403,
      json: { error: "File access is unavailable." },
    }),
  );
  await browser.getByRole("button", { name: "world", exact: true }).click();
  await expect(browser.getByRole("alert")).toContainText(
    "File access is unavailable.",
  );
  await expect(
    browser.getByRole("button", { name: "server.properties", exact: true }),
  ).toBeVisible();
  await expect(
    browser.getByText("This folder is empty.", { exact: true }),
  ).toHaveCount(0);
  await page.unroute("**/api/v1/servers/survival/files");
  await browser.getByRole("button", { name: "Try again", exact: true }).click();
  await expect(browser.getByRole("alert")).toHaveCount(0);
  await expect(
    browser.getByRole("button", { name: "level.dat", exact: true }),
  ).toBeVisible();
  await expect(
    browser.getByRole("button", { name: "server.properties", exact: true }),
  ).toHaveCount(0);
});
