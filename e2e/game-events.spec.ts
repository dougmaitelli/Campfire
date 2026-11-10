import { test, expect, prepareScreenshot } from "./fixtures";
import { login } from "./screens";

test("game activity reads retained events and filters by type and time", async ({
  page,
}) => {
  await login(page);
  await page.goto("/#servers/survival/activity");
  const feed = page.getByRole("region", { name: "Game activity", exact: true });
  await expect(feed.locator(".date-time-input-placeholder")).toHaveCount(2);
  for (const hint of await feed.locator(".date-time-input-placeholder").all())
    await expect(hint).toBeHidden();
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
    .getByRole("heading", { name: "Game activity", exact: true })
    .click();
  await prepareScreenshot(page);
  await expect(feed.locator("form")).toHaveScreenshot([
    "crops",
    "activity-date-filters-filled.png",
  ]);
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
  expect(
    (
      await page.request.get("/api/v1/servers/survival/events/filters")
    ).status(),
  ).toBe(404);
});

test("game activity combines player and mod-state filters and clears them", async ({
  page,
}) => {
  await login(page);
  await page.goto("/#servers/survival/activity");
  const feed = page.getByRole("region", { name: "Game activity", exact: true });
  const results = feed.locator(".game-event-list > li");
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
  await page.setViewportSize({ width: 2400, height: 900 });
  const fieldTops = await feed
    .locator("form .form-field")
    .evaluateAll((elements) =>
      elements.map((element) => element.getBoundingClientRect().top),
    );
  expect(new Set(fieldTops).size).toBe(1);
  await page.setViewportSize({ width: 1400, height: 900 });
  await feed
    .getByRole("button", { name: "Add field filter", exact: true })
    .click();
  await feed
    .getByRole("combobox", { name: "Event field", exact: true })
    .nth(1)
    .click();
  await expect(
    page.getByRole("option", { name: "Player name", exact: true }),
  ).toBeDisabled();
  await page
    .getByRole("option", { name: "Connection state", exact: true })
    .click();
  await feed
    .getByRole("combobox", { name: "Field value", exact: true })
    .nth(1)
    .click();
  await page.getByRole("option", { name: "connected", exact: true }).click();
  const request = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return (
      url.pathname.endsWith("/survival/events") &&
      url.searchParams.get("data.state") === "connected"
    );
  });
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  const query = new URL((await request).url()).searchParams;
  expect(query.get("search")).toBe("alex");
  expect(query.get("data.player")).toBe("Alex");
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
  await prepareScreenshot(page);
  await expect(feed.locator("form")).toHaveScreenshot([
    "crops",
    "activity-field-filters-mobile.png",
  ]);
  await page.setViewportSize({ width: 320, height: 844 });
  expect(
    await feed
      .locator("form")
      .evaluate((element) => element.scrollWidth <= element.clientWidth),
  ).toBe(true);
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

test("game activity keeps player filters when paging through older events", async ({
  page,
}) => {
  await login(page);
  await page.route("**/api/v1/servers/survival/events?*", async (route) => {
    const url = new URL(route.request().url());
    url.searchParams.set("limit", "1");
    const response = await route.fetch({ url: url.toString() });
    await route.fulfill({ response });
  });
  await page.goto("/#servers/survival/activity");
  const feed = page.getByRole("region", { name: "Game activity", exact: true });
  await feed.getByLabel("Search activity", { exact: true }).fill("Alex");
  await feed.getByRole("combobox", { name: "Event type", exact: true }).click();
  await expect(
    page.getByRole("option", { name: "player.joined", exact: true }),
  ).toBeVisible();
  await page.keyboard.press("Escape");
  await feed
    .getByRole("button", { name: "Add field filter", exact: true })
    .click();
  await feed
    .getByRole("combobox", { name: "Event field", exact: true })
    .click();
  await expect(
    page.getByRole("option", { name: "Connection state", exact: true }),
  ).toBeVisible();
  await page.getByRole("option", { name: "Player name", exact: true }).click();
  await feed
    .getByRole("combobox", { name: "Field value", exact: true })
    .click();
  await page.getByRole("option", { name: "Alex", exact: true }).click();
  await feed
    .getByRole("button", { name: "Apply filters", exact: true })
    .click();
  await expect(
    feed.getByText("Alex earned Stone Age", { exact: true }),
  ).toBeVisible();
  const request = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return (
      url.pathname.endsWith("/survival/events") &&
      url.searchParams.has("before")
    );
  });
  await feed.getByRole("button", { name: "Older events", exact: true }).click();
  const query = new URL((await request).url()).searchParams;
  expect(query.get("search")).toBe("Alex");
  expect(query.get("data.player")).toBe("Alex");
  await expect(
    feed.getByText("Alex joined the server", { exact: true }),
  ).toBeVisible();
  await expect(
    feed.getByRole("button", { name: "Older events", exact: true }),
  ).toHaveCount(0);
  await feed
    .getByRole("button", { name: "Newest events", exact: true })
    .click();
  await expect(
    feed.getByText("Alex earned Stone Age", { exact: true }),
  ).toBeVisible();
  await expect(
    feed.getByRole("combobox", { name: "Event field", exact: true }),
  ).toHaveText("Player name");
  await expect(
    feed.getByRole("combobox", { name: "Field value", exact: true }),
  ).toHaveText("Alex");
});
