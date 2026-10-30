import { test, expect } from "./fixtures";
import { login } from "./screens";

test("game activity reads retained events and filters by type and time", async ({
  page,
}) => {
  await login(page);
  await page.goto("/#servers/survival/activity");
  const feed = page.getByRole("region", { name: "Game activity", exact: true });
  await expect(
    feed.getByText("Alex joined the server", { exact: true }),
  ).toBeVisible();
  await expect(
    feed.getByText("Alex earned Stone Age", { exact: true }),
  ).toBeVisible();
  await feed.getByLabel("Event type", { exact: true }).fill("player.joined");
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(feed.locator(".game-event-list > li")).toHaveCount(1);
  await expect(
    feed.getByText("Alex joined the server", { exact: true }),
  ).toBeVisible();
  await expect(
    feed.getByText("Alex earned Stone Age", { exact: true }),
  ).toHaveCount(0);
  await feed
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await expect(feed.locator(".game-event-list > li")).toHaveCount(3);
  await feed
    .getByLabel("Occurred after", { exact: true })
    .fill("2030-01-01T00:00");
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(
    feed.getByRole("heading", { name: "No game events found", exact: true }),
  ).toBeVisible();
  const response = await page.request.get(
    "/api/v1/servers/survival/events?limit=1",
  );
  expect(response.ok()).toBeTruthy();
  const first = (await response.json()) as {
    items: { id: number }[];
    next_before: number;
  };
  expect(first.items).toHaveLength(1);
  const second = await page.request.get(
    `/api/v1/servers/survival/events?limit=1&before=${first.next_before}`,
  );
  const older = (await second.json()) as { items: { id: number }[] };
  expect(older.items[0]?.id).not.toBe(first.items[0]?.id);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(feed).toBeVisible();
});

test("unassigned users cannot read a server's game events", async ({
  page,
}) => {
  await login(page, "viewer");
  expect(
    (await page.request.get("/api/v1/servers/survival/events")).status(),
  ).toBe(404);
});
