import { test, expect } from "./fixtures";
import { login, openScreen } from "./screens";
import type { Page } from "@playwright/test";
import { readAuthenticatedSession } from "./auth";
import type { Page as Paginated } from "../web/src/api/pagination";
import type { Notification } from "../web/src/api/notifications";

async function prepareUnreadNotifications(page: Page) {
  const { csrf_token } = await readAuthenticatedSession(
    await page.request.get("/api/v1/auth/session"),
  );
  const response = await page.request.get("/api/v1/notifications");
  expect(response.ok()).toBeTruthy();
  const notifications = (await response.json()) as Paginated<Notification>;
  for (const title of ["Nightly backup completed", "Offline Spark"]) {
    const item = notifications.items.find((item) => item.title === title);
    if (!item) throw new Error(`Missing notification fixture: ${title}`);
    expect(
      (
        await page.request.put(`/api/v1/notifications/${item.id}/read`, {
          headers: {
            "X-CSRF-Token": csrf_token,
            Origin: new URL(page.url()).origin,
          },
          data: { read: false },
        })
      ).status(),
    ).toBe(204);
  }
}

test("notification rows acknowledge once without intercepting links", async ({
  page,
}) => {
  await login(page);
  await prepareUnreadNotifications(page);
  await openScreen(page, "notifications", "Notifications");
  const updates: unknown[] = [];
  page.on("request", (request) => {
    if (
      request.method() === "PUT" &&
      /\/notifications\/\d+\/read$/.test(request.url())
    )
      updates.push(request.postDataJSON());
  });
  const row = page.getByRole("article", {
    name: "Nightly backup completed",
    exact: true,
  });
  const check = row.getByRole("button", {
    name: "Mark read: Nightly backup completed",
    exact: true,
  });
  await page.mouse.move(0, 0);
  await expect(check).toHaveCSS("opacity", "0");
  await expect(row).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await row.hover();
  await expect(check).toHaveCSS("opacity", "1");
  await expect(row).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await row.getByRole("link").click();
  const details = page.getByRole("dialog", { name: "Server", exact: true });
  await expect(details).toBeVisible();
  expect(updates).toEqual([]);
  await details
    .getByRole("button", { name: "Close", exact: true })
    .first()
    .click();
  await expect(row.getByText("Unread", { exact: true })).toBeVisible();
  await row.locator("p").first().click();
  await expect(row.getByText("Unread", { exact: true })).toHaveCount(0);
  await expect(row.getByRole("button")).toHaveCount(0);
  await row.locator("p").first().click();
  expect(updates).toEqual([{ read: true }]);
  await expect(page.getByRole("button", { name: /^Mark unread:/ })).toHaveCount(
    0,
  );

  const next = page.getByRole("article", {
    name: "Offline Spark",
    exact: true,
  });
  const nextCheck = next.getByRole("button", {
    name: "Mark read: Offline Spark",
    exact: true,
  });
  await nextCheck.focus();
  await expect(nextCheck).toHaveCSS("opacity", "1");
  await expect(next).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await page.keyboard.press("Space");
  await expect(next.getByText("Unread", { exact: true })).toHaveCount(0);
  expect(updates).toEqual([{ read: true }, { read: true }]);
});

test.describe("touch notification acknowledgement", () => {
  test.use({
    hasTouch: true,
    isMobile: true,
    viewport: { width: 1400, height: 900 },
  });
  test("check affordance is visible without hover and rows can be tapped", async ({
    page,
  }) => {
    await login(page);
    await prepareUnreadNotifications(page);
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/#notifications");
    const row = page.getByRole("article", {
      name: "Nightly backup completed",
      exact: true,
    });
    await expect(
      row.getByRole("button", { name: "Mark read: Nightly backup completed" }),
    ).toHaveCSS("opacity", "1");
    await row.locator("p").first().tap();
    await expect(row.getByText("Unread", { exact: true })).toHaveCount(0);
    await expect(row.getByRole("button")).toHaveCount(0);
  });
});
