import { test, expect, screenshot } from "./fixtures";
import { login } from "./screens";
import { ExecutionState, ServerAction } from "../web/src/contracts/domains";

test("Activity groups a game event with its destination notifications and links both ways", async ({
  page,
  baseURL,
}) => {
  await login(page);
  const { csrf_token } = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const headers = { "X-CSRF-Token": csrf_token, Origin: baseURL! };
  expect(
    (
      await page.request.put("/api/v1/integrations/discord", {
        headers,
        data: {
          enabled: true,
          guild_id: "200",
          token: "campfire-test-discord-token",
        },
      })
    ).ok(),
  ).toBeTruthy();
  for (const channel_id of ["400", "402"])
    expect(
      (
        await page.request.post(
          "/api/v1/servers/survival/integrations/discord",
          {
            headers,
            data: {
              enabled: true,
              channel_id,
              subscriptions: [{ category: "game" }],
              card: { enabled: false, fields: [] },
            },
          },
        )
      ).ok(),
    ).toBeTruthy();
  expect(
    (
      await page.request.post("/__test/integrations/game-event", {
        headers: { "X-Campfire-E2E": "integration" },
        data: {
          server_id: "survival",
          event: {
            id: "activity-linked",
            type: "player.joined",
            title: "Morgan joined the game",
            occurred_at: "2026-10-22T12:00:00Z",
            data: { player: "Morgan" },
          },
        },
      })
    ).ok(),
  ).toBeTruthy();
  await page.goto("/#servers/survival/activity?search=Morgan");
  const activity = page.getByRole("region", { name: "Activity", exact: true });
  await expect(activity.locator("tbody tr")).toHaveCount(1);
  await expect(activity.locator("tbody tr")).toContainText("2");
  await screenshot(page, "server-activity.png", activity);
  await activity
    .getByRole("button", { name: "Morgan joined the game", exact: true })
    .click();
  const source = page.getByRole("dialog", {
    name: "Morgan joined the game",
    exact: true,
  });
  const notifications = source.getByRole("table", {
    name: "Notifications",
    exact: true,
  });
  await expect(notifications.locator("tbody tr")).toHaveCount(2);
  await expect(
    notifications.getByText("Delivered", { exact: true }),
  ).toHaveCount(2);
  await screenshot(page, "server-activity-event.png", source);
  await page.setViewportSize({ width: 390, height: 844 });
  await screenshot(page, "server-activity-event-mobile.png", source);
  await notifications
    .getByRole("button", { name: "Morgan joined the game", exact: true })
    .first()
    .click();
  const delivery = page.getByRole("dialog", {
    name: "Morgan joined the game",
    exact: true,
  });
  await expect(
    delivery.getByRole("link", {
      name: "Open message in Discord",
      exact: true,
    }),
  ).toHaveAttribute(
    "href",
    /^https:\/\/discord.com\/channels\/200\/(400|402)\/\d+$/,
  );
  await delivery
    .getByRole("button", { name: "View source activity", exact: true })
    .click();
  await expect(
    source
      .getByRole("table", { name: "Notifications", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);

  const sourceKey =
    new URLSearchParams(new URL(page.url()).hash.split("?")[1]).get("entry") ??
    "";
  expect(sourceKey).toMatch(/^game_event:\d+$/);
  // Exercise contextual pagination while keeping the real source records.
  await page.route(
    `**/api/v1/servers/survival/activity/${encodeURIComponent(sourceKey)}/notifications*`,
    async (route) => {
      if (
        new URL(route.request().url()).searchParams.get("before") ===
        "more-notifications"
      ) {
        await route.fulfill({ json: { items: [], next: "" } });
        return;
      }
      const response = await route.fetch();
      const notifications = (await response.json()) as {
        next: string;
      };
      notifications.next = "more-notifications";
      await route.fulfill({ response, json: notifications });
    },
  );
  await page.goto(
    "/#servers/survival/activity?kind=game&type=player.joined&data.player=Morgan",
  );
  await activity
    .getByRole("button", { name: "Morgan joined the game", exact: true })
    .click();
  await source
    .getByRole("button", { name: "Load older notifications", exact: true })
    .click();
  await expect(
    source.getByRole("button", {
      name: "Load older notifications",
      exact: true,
    }),
  ).toHaveCount(0);
  await expect(
    source
      .getByRole("table", { name: "Notifications", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
  await expect(page).toHaveURL(
    /kind=game&type=player.joined&data.player=Morgan&entry=game_event%3A\d+$/,
  );
});

test("an authorized user can retry one failed notification from Activity", async ({
  page,
  baseURL,
}) => {
  await login(page);
  const { csrf_token } = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const headers = { "X-CSRF-Token": csrf_token, Origin: baseURL! };
  expect(
    (
      await page.request.put("/api/v1/integrations/discord", {
        headers,
        data: {
          enabled: true,
          guild_id: "200",
          token: "campfire-test-discord-token",
        },
      })
    ).ok(),
  ).toBeTruthy();
  expect(
    (
      await page.request.post("/api/v1/servers/survival/integrations/discord", {
        headers,
        data: {
          enabled: true,
          channel_id: "400",
          subscriptions: [{ category: "game" }],
          card: { enabled: false, fields: [] },
        },
      })
    ).ok(),
  ).toBeTruthy();
  expect(
    (
      await page.request.post("/__test/integrations/game-event", {
        headers: { "X-Campfire-E2E": "integration" },
        data: {
          server_id: "survival",
          event: {
            id: "activity-retry",
            type: "player.joined",
            title: "Fixture rejected notification",
            occurred_at: "2026-10-22T12:00:00Z",
            data: { player: "Morgan" },
          },
        },
      })
    ).ok(),
  ).toBeTruthy();
  const before = Buffer.from(
    JSON.stringify({ at: 1893456000, key: "notification:1" }),
  ).toString("base64url");
  await page.goto(
    `/#servers/survival/activity?kind=game&search=Fixture+rejected&before=${before}`,
  );
  const originalPage = page.url();
  const activity = page.getByRole("region", { name: "Activity", exact: true });
  await expect(activity.locator("tbody tr")).toHaveCount(1);
  await activity
    .getByRole("button", { name: "Fixture rejected notification", exact: true })
    .click();
  const drawer = page.getByRole("dialog", {
    name: "Fixture rejected notification",
    exact: true,
  });
  await drawer
    .getByRole("table", { name: "Notifications", exact: true })
    .getByRole("button", { name: "Fixture rejected notification", exact: true })
    .click();
  await expect(
    drawer.getByRole("button", { name: "Try again", exact: true }),
  ).toBeVisible();
  await screenshot(page, "server-activity-notification.png", drawer);
  const retryURL = "**/api/v1/servers/survival/activity/*/retry";
  let release!: () => void;
  const holdRequest = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route(retryURL, async (route) => {
    await holdRequest;
    const response = await route.fetch();
    await route.fulfill({ response });
  });
  const retry = page.waitForRequest(retryURL);
  try {
    await drawer
      .getByRole("button", { name: "Try again", exact: true })
      .click();
    await retry;
    await page.goBack();
    await expect(
      drawer.getByRole("table", { name: "Notifications", exact: true }),
    ).toBeVisible();
    await page.goBack();
    await expect(drawer).toHaveCount(0);
    const newest = activity.getByRole("button", {
      name: "Newest activity",
      exact: true,
    });
    await newest.click();
    await expect(newest).toHaveCount(0);
    await page.goBack();
    await expect(page).toHaveURL(originalPage);
    const filteredPage = new URL(originalPage);
    const filterQuery = new URLSearchParams(filteredPage.hash.split("?")[1]);
    filterQuery.set("search", "Fixture rejected notification");
    filteredPage.hash = `#servers/survival/activity?${filterQuery}`;
    await page.goto(filteredPage.toString());
    await expect(activity.locator("tbody tr")).toContainText(
      "Fixture rejected notification",
    );
  } finally {
    release();
  }
  await activity
    .getByRole("button", { name: "Fixture rejected notification", exact: true })
    .click();
  const related = drawer.getByRole("table", {
    name: "Notifications",
    exact: true,
  });
  await expect(related.locator("tbody tr")).toContainText("Delivered");
  await related
    .getByRole("button", { name: "Fixture rejected notification", exact: true })
    .click();
  await expect(drawer.locator(".status-pill")).toHaveText("Delivered");
  await expect(
    drawer.getByRole("button", { name: "Try again", exact: true }),
  ).toHaveCount(0);
  await drawer
    .getByRole("button", { name: "View source activity", exact: true })
    .click();
  await expect(
    drawer
      .getByRole("table", { name: "Notifications", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(1);
  await page.goto("/mock/discord");
  await expect(
    page
      .getByRole("article")
      .filter({ hasText: "Fixture rejected notification" }),
  ).toHaveCount(1);
});

test("Activity confirms an operation retry and follows its new attempt", async ({
  page,
}) => {
  await login(page);
  const original = {
    id: "failed-restart",
    schedule_id: "",
    retry_of: "",
    recurring: false,
    server_id: "survival",
    server_name: "Survival",
    user_id: 1,
    username: "admin",
    action: ServerAction.Restart,
    state: ExecutionState.Failed,
    message: "Original restart failed.",
    created_at: 1792670400,
    started_at: 1792670400,
    finished_at: 1792670401,
  };
  const attempt = {
    ...original,
    id: "retried-restart",
    retry_of: original.id,
    state: ExecutionState.Running,
    message: "Retry in progress.",
    finished_at: 0,
  };
  const entry = (operation: typeof original | typeof attempt) => ({
    key: `operation:${operation.id}`,
    kind: "operation",
    title: "restart",
    state: operation.state,
    source_available: false,
    created_at: operation.created_at,
    notification_count: 0,
  });
  await page.route("**/api/v1/servers/survival/activity?*", (route) =>
    route.fulfill({ json: { items: [entry(original)], next: "" } }),
  );
  for (const operation of [original, attempt])
    await page.route(
      `**/api/v1/servers/survival/activity/operation%3A${operation.id}`,
      (route) =>
        route.fulfill({
          json: {
            ...entry(operation),
            operation,
            can_retry_operation: operation.id === original.id,
          },
        }),
    );
  let retries = 0;
  await page.route(
    "**/api/v1/schedules/executions/failed-restart/retry",
    (route) => {
      retries++;
      return route.fulfill({ status: 202, json: attempt });
    },
  );
  await page.goto(
    "/#servers/survival/activity?kind=operations&entry=operation%3Afailed-restart",
  );
  const drawer = page.getByRole("dialog", {
    name: "Server restart",
    exact: true,
  });
  await expect(
    drawer.getByRole("link", { name: "admin", exact: true }),
  ).toHaveAttribute("href", "#users/1");
  await drawer.getByRole("link", { name: "admin", exact: true }).click();
  const user = page.getByRole("dialog", { name: "Edit user", exact: true });
  await expect(user).toBeVisible();
  await expect(page).toHaveURL(/entry=operation%3Afailed-restart$/);
  await user.getByRole("button", { name: "Close", exact: true }).click();
  await drawer.getByRole("button", { name: "Try again", exact: true }).click();
  const confirmation = page.getByRole("dialog", {
    name: "Retry operation?",
    exact: true,
  });
  await expect(confirmation).toContainText(
    "an interrupted operation may already have executed",
  );
  await confirmation
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  expect(retries).toBe(0);
  await expect(drawer).toContainText(original.message);
  await drawer.getByRole("button", { name: "Try again", exact: true }).click();
  await confirmation
    .getByRole("button", { name: "Try again", exact: true })
    .click();
  await expect(page).toHaveURL(/entry=operation%3Aretried-restart$/);
  await expect(drawer).toContainText(attempt.message);
  await expect(
    drawer.getByRole("button", { name: "Try again", exact: true }),
  ).toHaveCount(0);
  expect(retries).toBe(1);
});
