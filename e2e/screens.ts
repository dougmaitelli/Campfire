import type { Page } from "@playwright/test";
import { expect } from "./fixtures";

export async function mockServerStatus(
  page: Page,
  serverID: string,
  read: () => Record<string, unknown>,
) {
  await page.route("**/api/v1/servers/status", async (route) => {
    const response = await route.fetch();
    const statuses = (await response.json()) as { server_id: string }[];
    await route.fulfill({
      response,
      json: statuses.map((status) =>
        status.server_id === serverID ? read() : status,
      ),
    });
  });
}

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

export async function openServerOperations(page: Page, name = "Survival") {
  await openScreen(page, "servers", "Servers");
  await page.getByRole("button", { name: `Open ${name}`, exact: true }).click();
  const serverID = page.url().split("#servers/")[1]!.split("/")[0]!;
  await page.goto(`/#servers/${serverID}/activity?kind=operations`);
  const activity = page.getByRole("region", { name: "Activity", exact: true });
  await expect(activity).toBeVisible();
  return activity;
}

export async function openServerJob(
  page: Page,
  serverID: string,
  jobID: string,
) {
  await page.goto(
    `/#servers/${encodeURIComponent(serverID)}/activity?kind=operations&entry=${encodeURIComponent(`job:${jobID}`)}`,
  );
  const drawer = page.getByRole("dialog");
  await expect(
    drawer.getByRole("row", { name: new RegExp(` ${jobID}$`) }),
  ).toBeVisible();
  return drawer;
}

export async function openServerActions(page: Page, name = "Survival") {
  await openScreen(page, "servers", "Servers");
  await page.getByRole("button", { name: `Open ${name}`, exact: true }).click();
  await expect(
    page.getByRole("heading", { name, level: 1, exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "More server actions", exact: true })
    .click();
  return page.getByRole("menu", { name: "More server actions", exact: true });
}

export async function openServerEditor(page: Page, name = "Creative") {
  const menu = await openServerActions(page, name);
  await menu
    .getByRole("menuitem", { name: "Edit server", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Edit server", exact: true });
  await expect(drawer.getByLabel("Server name", { exact: true })).toBeVisible();
  return drawer;
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
    .getByRole("button", { name: "Open Survival", exact: true })
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
  if (tab === "console") {
    await expect(
      page.getByRole("region", { name: "Console output", exact: true }),
    ).toContainText("Saved the game");
    await expect(page.getByText("Live", { exact: true })).toBeVisible();
    await expect(
      page.getByRole("textbox", { name: "Command", exact: true }),
    ).toBeVisible();
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
      page.getByRole("cell", { name: /^mock-nightly\.tar\.gz/ }),
    ).toBeVisible();
  }
}
