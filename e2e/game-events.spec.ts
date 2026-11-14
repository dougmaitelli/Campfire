import { test, expect, prepareScreenshot } from "./fixtures";
import { login } from "./screens";

test("game activity reads retained events and filters by type and recorded time", async ({
  page,
}) => {
  await login(page);
  await page.goto("/#servers/survival/activity?kind=game");
  const feed = page.getByRole("region", { name: "Activity", exact: true });
  const rows = feed.locator("tbody tr");
  await expect(rows).toHaveCount(3);
  await expect(
    feed.getByText("Alex joined the server", { exact: true }),
  ).toBeVisible();
  await expect(
    feed.getByText("Alex earned Stone Age", { exact: true }),
  ).toBeVisible();
  await prepareScreenshot(page);
  await expect(feed.locator("form")).toHaveScreenshot([
    "crops",
    "activity-date-filters.png",
  ]);
  await feed.getByRole("combobox", { name: "Event type", exact: true }).click();
  await page
    .getByRole("option", { name: "player.joined", exact: true })
    .click();
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(rows).toHaveCount(1);
  await expect(
    feed.getByText("Alex joined the server", { exact: true }),
  ).toBeVisible();
  await expect(
    feed.getByText("Alex earned Stone Age", { exact: true }),
  ).toHaveCount(0);
  await feed
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await expect(rows).toHaveCount(3);
  await feed
    .getByLabel("Recorded after", { exact: true })
    .fill("2030-01-01T00:00");
  await feed.getByRole("heading", { name: "Activity", exact: true }).click();
  await prepareScreenshot(page);
  await expect(feed.locator("form")).toHaveScreenshot([
    "crops",
    "activity-date-filters-filled.png",
  ]);
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(
    feed.getByRole("heading", {
      name: "No activity matches these filters.",
      exact: true,
    }),
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

test("unassigned users cannot read server activity, details or discovery", async ({
  page,
}) => {
  await login(page, "viewer");
  for (const path of [
    "events",
    "events/filters",
    "activity",
    "activity/filters",
    "activity/game_event:1",
  ])
    expect(
      (await page.request.get(`/api/v1/servers/survival/${path}`)).status(),
    ).toBe(404);
});

test("game activity combines player and mod-state filters and clears them", async ({
  page,
}) => {
  await login(page);
  await page.goto("/#servers/survival/activity?kind=game");
  const feed = page.getByRole("region", { name: "Activity", exact: true });
  const results = feed.locator("tbody tr");
  await expect(results).toHaveCount(3);
  await expect(
    feed.getByRole("combobox", { name: "Event field", exact: true }),
  ).toHaveCount(0);
  await feed.getByLabel("Search activity", { exact: true }).fill("alex");
  await feed
    .getByRole("button", { name: "Add field filter", exact: true })
    .click();
  await feed
    .getByRole("combobox", { name: "Event field", exact: true })
    .click();
  await expect(page.getByRole("option")).toHaveText([
    "Select a field",
    "Advancement",
    "Game",
    "Player name",
    "Connection state",
  ]);
  await page.getByRole("option", { name: "Player name", exact: true }).click();
  await feed
    .getByRole("combobox", { name: "Field value", exact: true })
    .click();
  await page.getByRole("option", { name: "Alex", exact: true }).click();
  await feed
    .getByRole("button", { name: "Add field filter", exact: true })
    .click();
  await feed
    .getByRole("combobox", { name: "Event field", exact: true })
    .nth(1)
    .click();
  await page
    .getByRole("option", { name: "Connection state", exact: true })
    .click();
  await feed
    .getByRole("combobox", { name: "Field value", exact: true })
    .nth(1)
    .click();
  await page.getByRole("option", { name: "connected", exact: true }).click();
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(results).toHaveCount(1);
  await expect(
    feed.getByText("Alex joined the server", { exact: true }),
  ).toBeVisible();
  await prepareScreenshot(page);
  await expect(feed.locator("form")).toHaveScreenshot([
    "crops",
    "activity-field-filters.png",
  ]);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(feed.locator("form")).toHaveScreenshot([
    "crops",
    "activity-field-filters-mobile.png",
  ]);
  await feed
    .getByRole("button", { name: "Remove field filter 2", exact: true })
    .click();
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(results).toHaveCount(2);
  await feed
    .getByRole("combobox", { name: "Event field", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Connection state", exact: true })
    .click();
  await expect(
    feed.getByRole("combobox", { name: "Field value", exact: true }),
  ).toHaveText("Any value");
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(results).toHaveCount(1);
  await feed
    .getByRole("button", { name: "Clear filters", exact: true })
    .click();
  await expect(results).toHaveCount(3);
  await expect(feed.getByLabel("Search activity", { exact: true })).toHaveValue(
    "",
  );
  await expect(
    feed.getByRole("combobox", { name: "Event field", exact: true }),
  ).toHaveCount(0);
  await feed
    .getByRole("button", { name: "Add field filter", exact: true })
    .click();
  await expect(
    feed.getByRole("combobox", { name: "Event field", exact: true }),
  ).toHaveText("Select a field");
  await expect(
    feed.getByRole("combobox", { name: "Field value", exact: true }),
  ).toHaveText("Any value");
  await expect(
    feed.getByRole("combobox", { name: "Field value", exact: true }),
  ).toBeDisabled();
  await feed
    .getByRole("button", { name: "Remove field filter 1", exact: true })
    .click();
  await expect(
    feed.getByRole("combobox", { name: "Event field", exact: true }),
  ).toHaveCount(0);
});

test("game activity keeps field filters while paging through a live history", async ({
  page,
}) => {
  await login(page);
  for (let index = 0; index < 53; index++)
    expect(
      (
        await page.request.post("/__test/integrations/game-event", {
          headers: { "X-Campfire-E2E": "integration" },
          data: {
            server_id: "survival",
            event: {
              id: `page-${index}`,
              type: "game.capture",
              title: `Alex capture ${index}`,
              occurred_at: "2026-10-22T12:00:00Z",
              data: { player: "Alex" },
              field_labels: { player: "Player name" },
            },
          },
        })
      ).ok(),
    ).toBeTruthy();
  await page.goto(
    "/#servers/survival/activity?kind=game&search=Alex&data.player=Alex",
  );
  const feed = page.getByRole("region", { name: "Activity", exact: true });
  await expect(feed.locator("tbody tr")).toHaveCount(50);
  await expect(
    feed.getByRole("combobox", { name: "Event field", exact: true }),
  ).toHaveText("Player name");
  const request = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return (
      url.pathname.endsWith("/survival/activity") &&
      url.searchParams.has("before")
    );
  });
  await feed
    .getByRole("button", { name: "Older activity", exact: true })
    .click();
  const query = new URL((await request).url()).searchParams;
  expect(query.get("search")).toBe("Alex");
  expect(query.get("data.player")).toBe("Alex");
  await expect(feed.locator("tbody tr")).toHaveCount(5);
  await expect(
    feed.getByRole("button", { name: "Older activity", exact: true }),
  ).toHaveCount(0);
  await expect(
    feed.getByRole("combobox", { name: "Field value", exact: true }),
  ).toHaveText("Alex");
  await feed
    .getByRole("button", { name: "Newest activity", exact: true })
    .click();
  await expect(feed.locator("tbody tr")).toHaveCount(50);
});
