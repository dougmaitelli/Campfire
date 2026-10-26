import type { Page } from "@playwright/test";
import { expect } from "./fixtures";

export async function login(page: Page, username = "admin") {
  await page.goto("/");
  await page.getByLabel("Username", { exact: true }).fill(username);
  await page
    .getByLabel("Password", { exact: true })
    .fill("campfire-e2e-password");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("navigation", { name: "Main navigation" }),
  ).toBeVisible();
  expect(
    (
      await page.request.post("/__test/reset?keep_session=true", {
        headers: { "X-Campfire-E2E": "reset" },
      })
    ).ok(),
  ).toBeTruthy();
  await page.reload();
  await expect(
    page.getByRole("navigation", { name: "Main navigation" }),
  ).toBeVisible();
}

export const screens = [
  ["dashboard", "Overview"],
  ["servers", "Servers"],
  ["sparks", "Sparks"],
  ["embers", "Embers"],
  ["schedules", "Schedules"],
  ["users", "Users"],
  ["roles", "Roles"],
  ["backups", "Backups"],
  ["audit", "Audit"],
  ["notifications", "Notifications"],
] as const;

export async function openScreen(page: Page, id: string, heading: string) {
  await page.setViewportSize({ width: 1400, height: 900 });
  const navigation =
    id === "notifications"
      ? page.getByRole("main")
      : page.getByRole("navigation", { name: "Main navigation", exact: true });
  await navigation
    .getByRole("link", { name: heading, exact: id !== "notifications" })
    .click();
  await expect(page).toHaveURL(new RegExp(`#${id}$`));
  await expect(
    page.getByRole("heading", { level: 1, name: heading, exact: true }),
  ).toBeVisible();
  await expect(page.getByText(/Loading[.…]/)).toHaveCount(0);
  const caption = page.locator("caption.visually-hidden").first();
  if (await caption.count())
    await expect(caption).toHaveCSS("position", "absolute");
  if (id === "backups") await expect(page.locator("tbody tr")).toHaveCount(8);
  if (id === "notifications") {
    await expect(
      page.getByText("Nightly backup completed", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText("Offline Spark", { exact: true }),
    ).toBeVisible();
  }
  if (id === "roles") {
    await expect(
      page.getByRole("row").filter({ hasText: "Community managers" }),
    ).toBeVisible();
    await expect(
      page.getByRole("row").filter({ hasText: "World maintainers" }),
    ).toBeVisible();
  }
  if (id === "embers") {
    await expect(
      page.locator('img[src="/game-art/mock-block.svg"]'),
    ).toBeVisible();
    await expect
      .poll(() =>
        page
          .locator('img[src="/game-art/mock-block.svg"]')
          .evaluate((image) => (image as HTMLImageElement).naturalWidth),
      )
      .toBeGreaterThan(0);
  }
  if (id === "dashboard") {
    const tiles = page.locator(".metric-tiles > *");
    await expect(tiles).toHaveCount(6);
    const rows = await tiles.evaluateAll((elements) =>
      elements.map((element) =>
        Math.round(element.getBoundingClientRect().top),
      ),
    );
    expect(
      new Set(rows).size,
      "Overview summary tiles stay on one row at desktop width",
    ).toBe(1);
  }
}

export async function openUser(page: Page) {
  await openScreen(page, "users", "Users");
  await page
    .getByRole("button", { name: "Edit user Developer", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
}

export async function inspectServer(page: Page) {
  await openScreen(page, "schedules", "Schedules");
  await page
    .getByRole("link", { name: "Survival", exact: true })
    .first()
    .click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(
    page.getByRole("dialog").getByText("Community Minecraft server"),
  ).toBeVisible();
}

export const serverTabs = [
  ["console", "Console"],
  ["files", "Files"],
  ["backups", "Backups"],
] as const;

export async function openServerTab(page: Page, tab: string, label: string) {
  await openScreen(page, "servers", "Servers");
  await page.setViewportSize({ width: 1400, height: 1100 });
  await page
    .getByRole("link", { name: "Open console for Survival", exact: true })
    .click();
  const tabs = page.getByRole("navigation", {
    name: "Server operations",
    exact: true,
  });
  await tabs.getByRole("link", { name: label, exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`#servers/survival/${tab}$`));
  await expect(
    page.getByRole("heading", { level: 1, name: "Survival", exact: true }),
  ).toBeVisible();
  await expect(page.locator('[aria-busy="true"]')).toHaveCount(0);
  await expect
    .poll(() =>
      tabs.evaluate((element) => element.scrollHeight <= element.clientHeight),
    )
    .toBe(true);
  if (tab === "console") {
    await expect(
      page.getByRole("region", { name: "Console output", exact: true }),
    ).toContainText("Saved the game");
    await expect(page.getByText("Live", { exact: true })).toBeVisible();
    await expect(
      page.getByRole("textbox", { name: "Command", exact: true }),
    ).toBeInViewport({ ratio: 1 });
  } else if (tab === "files") {
    await expect(
      page.getByRole("button", { name: "server.properties", exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "world", exact: true }),
    ).toBeVisible();
  } else {
    await expect(page.locator("tbody tr")).toHaveCount(2);
    await expect(
      page.getByRole("cell", { name: "mock-nightly.tar.gz", exact: true }),
    ).toBeVisible();
  }
}
