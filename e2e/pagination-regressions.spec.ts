import type { Page } from "@playwright/test";
import { test, expect, pageFixture } from "./fixtures";
import { login } from "./screens";

async function mockNotificationPages(page: Page) {
  const requests: number[] = [];
  await page.route("**/api/v1/notifications?**", (route) => {
    const number = Number(
      new URL(route.request().url()).searchParams.get("page") ?? 1,
    );
    requests.push(number);
    return route.fulfill({
      json: pageFixture(
        [
          {
            id: number === 1 ? 101 : 51,
            target_type: "account",
            target_id: "1",
            kind: "test",
            severity: "info",
            title: `Page ${number} message`,
            body: "Notification pagination regression",
            created_at: 1,
            read_at: 0,
          },
        ],
        number,
        101,
      ),
    });
  });
  return requests;
}

test("Audit Back and Forward restore URL filters rather than obsolete drafts", async ({
  page,
}) => {
  await page.route("**/api/v1/audit?**", (route) => {
    const number = Number(
      new URL(route.request().url()).searchParams.get("page") ?? 1,
    );
    return route.fulfill({ json: pageFixture([], number, 100) });
  });
  await login(page);
  await page.goto("/#audit?page=2");
  const action = page.getByRole("textbox", { name: "Action", exact: true });
  const target = page.getByRole("textbox", { name: "Target", exact: true });
  await action.fill("server.restart");
  await target.fill("server:creative");
  await page.getByRole("button", { name: "Filter", exact: true }).click();
  await expect(page).toHaveURL(
    /#audit\?action=server.restart&target=server%3Acreative$/,
  );
  await action.fill("unsaved");
  await page.goBack();
  await expect(page).toHaveURL(/#audit\?page=2$/);
  await expect(action).toHaveValue("");
  await expect(target).toHaveValue("");
  await page.goForward();
  await expect(action).toHaveValue("server.restart");
  await expect(target).toHaveValue("server:creative");
});

test("Mark all read on a later notification page sends the newest inbox watermark", async ({
  page,
}) => {
  await mockNotificationPages(page);
  let watermark = 0;
  await page.route("**/api/v1/notifications/read-all", (route) => {
    watermark = route.request().postDataJSON().through_id;
    return route.fulfill({ status: 204 });
  });
  await login(page);
  await page.goto("/#notifications?page=2");
  await expect(
    page.getByRole("article", { name: "Page 2 message" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Mark all read", exact: true })
    .click();
  await expect.poll(() => watermark).toBe(101);
  await expect(page).toHaveURL(/#notifications\?page=2$/);
  await expect(
    page.getByRole("article", { name: "Page 2 message" }),
  ).toBeVisible();
  await expect(
    page.getByRole("article", { name: "Page 1 message" }),
  ).toHaveCount(0);
});

test("a pending notification mutation refreshes the page restored by browser Back", async ({
  page,
}) => {
  const requests = await mockNotificationPages(page);
  let finish!: () => void;
  const completed = new Promise<void>((resolve) => {
    finish = resolve;
  });
  let pending = false;
  await page.route("**/api/v1/notifications/51/read", async (route) => {
    pending = true;
    await completed;
    await route.fulfill({ status: 204 });
  });
  await login(page);
  await page.goto("/#notifications");
  await expect(
    page.getByRole("article", { name: "Page 1 message" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await page
    .getByRole("button", { name: "Mark read: Page 2 message", exact: true })
    .click();
  await expect.poll(() => pending).toBe(true);
  await page.goBack();
  await expect(
    page.getByRole("article", { name: "Page 1 message" }),
  ).toBeVisible();
  requests.length = 0;
  finish();
  await expect.poll(() => requests.length).toBeGreaterThan(0);
  expect(requests.every((number) => number === 1)).toBe(true);
  await expect(
    page.getByRole("article", { name: "Page 1 message" }),
  ).toBeVisible();
});
