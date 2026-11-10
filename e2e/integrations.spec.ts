import { test, expect, screenshot } from "./fixtures";
import { login } from "./screens";
import {
  openServerIntegrations,
  editServerIntegration,
  saveServerIntegration,
} from "./integration-screens";
import type { GameStateView } from "../web/src/api/gameState";

type ConnectionChoices = {
  connections: {
    id: string;
    provider: string;
    name: string;
    enabled: boolean;
  }[];
};

test("alert settings discovers game-card fields and limits selection before saving", async ({
  page,
  baseURL,
}) => {
  test.setTimeout(60_000);
  await login(page);
  const session = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  expect(
    (
      await page.request.put("/api/v1/integrations/discord", {
        headers: { "X-CSRF-Token": session.csrf_token, Origin: baseURL! },
        data: {
          enabled: true,
          guild_id: "200",
          token: "campfire-test-discord-token",
        },
      })
    ).ok(),
  ).toBeTruthy();
  await page.route("**/api/v1/servers/survival/log-alerts", (route) =>
    route.fulfill({
      json: [
        {
          id: "1".repeat(64),
          rule_key: "2".repeat(64),
          execution_id: "3".repeat(64),
          phase: "runtime",
          title: "Review server settings",
          message: "Check the server configuration.",
          severity: "warning",
          detected_at: 1792670400,
          action: { type: "server_settings" },
        },
      ],
    }),
  );
  await page.route("**/api/v1/monitoring", async (route) => {
    const response = await route.fetch();
    const snapshot = (await response.json()) as {
      collected_at: number;
      servers: { id: string; game?: GameStateView }[];
    };
    const server = snapshot.servers.find((item) => item.id === "survival");
    if (!server) throw new Error("Missing server fixture");
    server.game = {
      updated_at: snapshot.collected_at,
      fresh_for_seconds: 60,
      status: "fresh",
      state: {
        players: 0,
        capacity: 20,
        fields: Object.fromEntries(
          Array.from({ length: 32 }, (_, i) => [`key${i}`, i]),
        ),
      },
    };
    await route.fulfill({ response, json: snapshot });
  });
  await page.goto("/#servers/survival/console");
  await page
    .getByRole("button", { name: "Server settings", exact: true })
    .click();
  const drawer = page.getByRole("dialog", { name: "Edit server", exact: true });
  await drawer
    .getByRole("button", { name: "Integrations", exact: true })
    .click();
  await drawer
    .getByRole("button", { name: "Add integration", exact: true })
    .click();
  await drawer
    .getByRole("combobox", { name: "Discord channel", exact: true })
    .click();
  await page
    .getByRole("option", { name: "#server-updates", exact: true })
    .click();
  await drawer
    .getByRole("checkbox", { name: "Show a live server card", exact: true })
    .check();
  for (const name of [
    "players",
    "capacity",
    ...Array.from({ length: 30 }, (_, i) => `key${i}`),
  ]) {
    await drawer.getByRole("checkbox", { name, exact: true }).check();
  }
  await expect(
    drawer.getByRole("checkbox", { name: "key30", exact: true }),
  ).toBeDisabled();
  await expect(
    drawer.getByRole("checkbox", { name: "key31", exact: true }),
  ).toBeDisabled();
  await drawer
    .getByRole("checkbox", { name: "players", exact: true })
    .uncheck();
  await drawer.getByRole("checkbox", { name: "key30", exact: true }).check();
  await expect(
    drawer.getByRole("checkbox", { name: "key31", exact: true }),
  ).toBeDisabled();
  const saved = page.waitForResponse(
    (response) =>
      response.request().method() === "PUT" &&
      response.url().endsWith("/servers/survival/integrations/discord"),
  );
  await saveServerIntegration(drawer);
  const values = (await (await saved).json()) as {
    card: { fields: string[] };
  }[];
  expect(values[0]!.card.fields).toHaveLength(32);
  expect(values[0]!.card.fields).toContain("field.key30");
  expect(values[0]!.card.fields).not.toContain("players");
});

test("server integrations require a saved connection when several are available", async ({
  page,
  baseURL,
}) => {
  await login(page);
  const session = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  expect(
    (
      await page.request.put("/api/v1/integrations/discord", {
        headers: { "X-CSRF-Token": session.csrf_token, Origin: baseURL! },
        data: {
          enabled: true,
          guild_id: "200",
          token: "campfire-test-discord-token",
        },
      })
    ).ok(),
  ).toBeTruthy();
  const base = "/api/v1/servers/survival/integrations/discord";
  const initial = (await (
    await page.request.get(base)
  ).json()) as ConnectionChoices;
  const savedConnection = initial.connections[0]!;
  // Exercise the future multiple-connection UI without expanding global bot setup.
  await page.route(`**${base}`, async (route) => {
    if (route.request().method() !== "GET") return route.continue();
    const response = await route.fetch();
    const value = (await response.json()) as ConnectionChoices;
    await route.fulfill({
      response,
      json: {
        ...value,
        connections: [
          ...value.connections,
          {
            id: "another-connection",
            provider: "discord",
            name: "Admin Community",
            enabled: true,
          },
        ],
      },
    });
  });
  await page.route(`**${base}/channels?*`, async (route) => {
    if (
      new URL(route.request().url()).searchParams.get("integration_id") !==
      "another-connection"
    )
      return route.continue();
    await route.fulfill({
      json: [
        { id: "999", name: "admin-channel", available: true, missing: [] },
      ],
    });
  });
  await page.goto("/#servers/survival/console");
  const drawer = await openServerIntegrations(page);
  await drawer
    .getByRole("button", { name: "Add integration", exact: true })
    .click();
  await page.setViewportSize({ width: 320, height: 844 });
  const connection = drawer.getByRole("combobox", {
    name: "Integration",
    exact: true,
  });
  const channel = drawer.getByRole("combobox", {
    name: "Discord channel",
    exact: true,
  });
  const save = drawer.getByRole("button", {
    name: "Next: Access",
    exact: true,
  });
  await expect(connection).toHaveText("Choose an integration");
  await expect(channel).toHaveCount(0);
  await expect(save).toBeDisabled();
  await connection.click();
  await page
    .getByRole("option", { name: "Discord · Admin Community", exact: true })
    .click();
  await channel.click();
  await page
    .getByRole("option", { name: "#admin-channel", exact: true })
    .click();
  await connection.click();
  await page
    .getByRole("option", { name: "Discord · Campfire Community", exact: true })
    .click();
  await expect(channel).toHaveText("Choose a channel");
  await expect(save).toBeDisabled();
  await channel.click();
  await page
    .getByRole("option", { name: "#server-updates", exact: true })
    .click();
  const request = page.waitForRequest(
    (request) => request.method() === "PUT" && request.url().endsWith(base),
  );
  await saveServerIntegration(drawer);
  expect((await request).postDataJSON()).toMatchObject({
    upserts: [
      {
        id: expect.any(String),
        settings: { integration_id: savedConnection.id, channel_id: "400" },
      },
    ],
    remove_ids: [],
  });
  await editServerIntegration(page);
  await expect(connection).toHaveText("Discord · Campfire Community");
  await expect(connection).toBeDisabled();
});

test("per-server Discord settings persist, enforce channel access, and keep card choices independent", async ({
  page,
  baseURL,
}) => {
  test.setTimeout(90_000);
  await login(page);
  const { csrf_token } = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  expect(
    (
      await page.request.put("/api/v1/integrations/discord", {
        headers: { "X-CSRF-Token": csrf_token, Origin: baseURL! },
        data: {
          enabled: true,
          guild_id: "200",
          token: "campfire-test-discord-token",
        },
      })
    ).ok(),
  ).toBeTruthy();
  let hasGameState = true;
  let hasFilterChoices = true;
  await page.route(
    "**/api/v1/servers/survival/events/filters",
    async (route) => {
      if (hasFilterChoices) return route.continue();
      await route.fulfill({ json: { types: [], fields: {} } });
    },
  );
  await page.route("**/api/v1/monitoring", async (route) => {
    const response = await route.fetch();
    const snapshot = (await response.json()) as {
      collected_at: number;
      servers: { id: string; game?: GameStateView }[];
    };
    const server = snapshot.servers.find((item) => item.id === "survival");
    if (!server) throw new Error("Missing server fixture");
    if (hasGameState) {
      server.game = {
        updated_at: snapshot.collected_at,
        fresh_for_seconds: 60,
        status: "fresh",
        state: {
          players: 3,
          capacity: 20,
          fields: { invite_code: "test-invite", map: "World" },
        },
      };
    } else delete server.game;
    await route.fulfill({ response, json: snapshot });
  });
  await page.route(
    "**/api/v1/servers/survival/integrations/discord/card?*",
    async (route) => {
      const response = await route.fetch();
      const card = (await response.json()) as {
        state: string;
        expires_at: number;
        addresses: string[];
        fields: { id: string; value: string; available: boolean }[];
      };
      if (hasGameState) {
        card.state = "running";
        card.expires_at = Math.floor(
          new Date("2026-10-22T12:01:00Z").getTime() / 1000,
        );
        card.addresses = ["games.example.test:25565 / TCP"];
        const values: Record<string, string> = {
          players: "3",
          capacity: "20",
          "field.invite_code": "test-invite",
          "field.map": "World",
        };
        for (const field of card.fields) {
          field.value = values[field.id] ?? "";
          field.available = field.id in values;
        }
      }
      await route.fulfill({ response, json: card });
    },
  );
  await page.goto("/#servers/survival/console");
  const open = () => editServerIntegration(page);
  await open();
  const drawer = page.getByRole("dialog", {
    name: "Edit server",
    exact: true,
  });
  const integration = drawer.getByRole("combobox", {
    name: "Integration",
    exact: true,
  });
  await expect(integration).toHaveText("Discord · Campfire Community");
  await integration.click();
  await page
    .getByRole("option", { name: "Discord · Campfire Community", exact: true })
    .click();
  const save = drawer.getByRole("button", {
    name: "Next: Access",
    exact: true,
  });
  await expect(
    drawer.getByRole("region", { name: "Card publishing", exact: true }),
  ).toHaveCount(0);
  await drawer.getByRole("switch", { name: "Enabled", exact: true }).check();
  const channel = drawer.getByRole("combobox", {
    name: "Discord channel",
    exact: true,
  });
  await channel.click();
  await page
    .getByRole("option", {
      name: "#read-only (missing permissions)",
      exact: true,
    })
    .click();
  await expect(drawer).toContainText("Missing bot permissions: Send Messages.");
  await expect(save).toBeDisabled();
  await channel.click();
  await page
    .getByRole("option", { name: "#server-updates", exact: true })
    .click();
  for (const name of [
    "Game events",
    "Power events",
    "Console alerts",
    "Backups",
    "Show a live server card",
    "players",
    "invite_code",
  ])
    await drawer.getByRole("checkbox", { name, exact: true }).check();
  await expect(
    drawer.getByText("Save changes to publish this card.", { exact: true }),
  ).toBeVisible();
  await expect(
    drawer.getByRole("combobox", { name: "Event field", exact: true }),
  ).toHaveCount(0);
  await drawer
    .getByRole("combobox", { name: "Event type", exact: true })
    .click();
  await page
    .getByRole("option", { name: "player.joined", exact: true })
    .click();
  await drawer
    .getByRole("button", { name: "Add event type", exact: true })
    .click();
  await drawer
    .getByRole("combobox", { name: "Event type", exact: true })
    .nth(1)
    .click();
  await page
    .getByRole("option", { name: "minecraft.advancement", exact: true })
    .click();
  await drawer
    .getByRole("group", { name: "Event type 1", exact: true })
    .getByRole("button", { name: "Add field filter", exact: true })
    .click();
  await drawer
    .getByRole("combobox", { name: "Event field", exact: true })
    .click();
  await page.getByRole("option", { name: "player", exact: true }).click();
  await drawer
    .getByRole("combobox", { name: "Field value", exact: true })
    .click();
  await page.getByRole("option", { name: "Alex", exact: true }).click();
  const creation = page.waitForResponse(
    (response) =>
      response.request().method() === "PUT" &&
      response.url().endsWith("/servers/survival/integrations/discord"),
  );
  await saveServerIntegration(drawer);
  await open();
  expect(((await (await creation).json()) as { id: string }[])[0]!.id).not.toBe(
    "",
  );
  const cardPreview = drawer.getByRole("region", {
    name: "Server card preview",
    exact: true,
  });
  await expect(cardPreview).toContainText("games.example.test:25565 / TCP");
  await expect(cardPreview).toContainText("test-invite");
  await expect(cardPreview).not.toContainText("capacity");
  const publishing = drawer.getByRole("region", {
    name: "Card publishing",
    exact: true,
  });
  await expect(
    publishing.getByText("Published", { exact: true }),
  ).toBeVisible();
  await screenshot(page, "integrations-server.png", drawer);
  hasGameState = false;
  await expect(page.locator(".game-state-readings")).toHaveCount(0);
  await expect(
    drawer.getByRole("checkbox", { name: "map", exact: true }),
  ).toBeVisible();
  await expect(cardPreview).toContainText("Unavailable");
  hasGameState = true;
  await page.clock.setFixedTime(new Date("2026-10-22T12:01:01Z"));
  await expect(cardPreview).not.toContainText("test-invite");
  await page.clock.setFixedTime(new Date("2026-10-22T12:00:00Z"));
  await expect(cardPreview).toContainText("test-invite");
  await expect(
    drawer.getByRole("region", { name: "Notification history", exact: true }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
  hasGameState = false;
  hasFilterChoices = false;
  await page.reload();
  await open();
  await expect(
    drawer.getByRole("switch", { name: "Enabled", exact: true }),
  ).toBeChecked();
  await expect(channel).toHaveText("#server-updates");
  await expect(
    drawer.getByRole("combobox", { name: "Event type", exact: true }).nth(0),
  ).toHaveText("player.joined");
  await expect(
    drawer.getByRole("combobox", { name: "Event type", exact: true }).nth(1),
  ).toHaveText("minecraft.advancement");
  await drawer
    .getByRole("button", { name: "Remove event type 2", exact: true })
    .click();
  await expect(
    drawer.getByRole("combobox", { name: "Event field", exact: true }),
  ).toHaveText("player");
  await expect(
    drawer.getByRole("combobox", { name: "Field value", exact: true }),
  ).toHaveText("Alex");
  await expect(
    drawer.getByRole("checkbox", { name: "players", exact: true }),
  ).toBeChecked();
  await expect(
    drawer.getByRole("checkbox", { name: "invite_code", exact: true }),
  ).toBeChecked();
  await expect(
    drawer.getByRole("checkbox", { name: "map", exact: true }),
  ).toHaveCount(0);
  await expect(cardPreview).toContainText("Unavailable");
  await expect(cardPreview).not.toContainText("test-invite");
  await expect(
    drawer.getByRole("checkbox", { name: "capacity", exact: true }),
  ).toHaveCount(0);
  const playerCount = drawer.getByRole("checkbox", {
    name: "players",
    exact: true,
  });
  await playerCount.uncheck();
  await expect(cardPreview).not.toContainText("players");
  await playerCount.check();
  await expect(cardPreview).toContainText("players");
  await expect(cardPreview).not.toContainText("test-invite");
  await page.setViewportSize({ width: 320, height: 844 });
  await cardPreview.scrollIntoViewIfNeeded();
  await expect(cardPreview).toBeInViewport({ ratio: 0.99 });
  expect(
    await drawer.evaluate(
      (element) => element.scrollWidth <= element.clientWidth,
    ),
  ).toBe(true);
  await channel.click();
  await page
    .getByRole("option", { name: "#other-updates", exact: true })
    .click();
  await saveServerIntegration(drawer);
  await open();
  await expect(publishing.getByText("Published", { exact: true })).toBeVisible({
    timeout: 20_000,
  });
  await drawer
    .getByRole("checkbox", { name: "Show a live server card", exact: true })
    .uncheck();
  await expect(
    drawer.getByRole("checkbox", { name: "Game events", exact: true }),
  ).toBeChecked();
  await saveServerIntegration(drawer);
  await open();
  await page.setViewportSize({ width: 320, height: 844 });
  await expect(publishing).toHaveCount(0);
  await expect(save).toBeInViewport({ ratio: 1 });
  await drawer.getByRole("switch", { name: "Enabled", exact: true }).uncheck();
  await saveServerIntegration(drawer);
  await open();
  await expect(
    drawer.getByRole("checkbox", { name: "Game events", exact: true }),
  ).toBeChecked();
  const other = await page.request.get(
    "/api/v1/servers/creative/integrations/discord",
  );
  expect(((await other.json()) as { items: unknown[] }).items).toHaveLength(0);
});

for (const scenario of [
  {
    suffix: "retry-card",
    state: "Failed",
    action: "Retry card",
    result: "Published",
  },
  {
    suffix: "unconfirmed-card",
    state: "Unconfirmed",
    action: "Check card",
    result: "Unconfirmed",
  },
] as const) {
  test(`card publishing offers ${scenario.action} for ${scenario.state.toLowerCase()} cards`, async ({
    page,
    baseURL,
  }) => {
    test.setTimeout(60_000);
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
            token: `campfire-test-discord-token-${scenario.suffix}`,
          },
        })
      ).ok(),
    ).toBeTruthy();
    expect(
      (
        await page.request.post(
          "/api/v1/servers/survival/integrations/discord",
          {
            headers,
            data: {
              enabled: true,
              channel_id: "400",
              subscriptions: [],
              card: { enabled: true, fields: ["players"] },
            },
          },
        )
      ).ok(),
    ).toBeTruthy();
    await page.goto("/#servers/survival/console");
    await openServerIntegrations(page);
    await page
      .getByRole("button", {
        name: "Edit integration for #server-updates",
        exact: true,
      })
      .click();
    const publishing = page.getByRole("region", {
      name: "Card publishing",
      exact: true,
    });
    await expect(
      publishing.getByText(scenario.state, { exact: true }),
    ).toBeVisible({ timeout: 20_000 });
    const action = publishing.getByRole("button", {
      name: scenario.action,
      exact: true,
    });
    await expect(action).toBeVisible({ timeout: 20_000 });
    const response = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/integrations\/discord\/[^/]+\/card\/retry$/.test(response.url()),
    );
    await action.click();
    expect((await response).status()).toBe(204);
    await expect(
      publishing.getByText(scenario.result, { exact: true }),
    ).toBeVisible({ timeout: 20_000 });
    if (scenario.result === "Unconfirmed") {
      await expect(action).toBeVisible({ timeout: 15_000 });
      await expect(publishing).toContainText(
        "Automatic creation is paused to avoid duplicates.",
      );
    } else await expect(action).toHaveCount(0);
  });
}

for (const access of ["viewer", "editor"] as const) {
  test(`server integrations follow ${access} server access`, async ({
    page,
    baseURL,
  }) => {
    await login(page);
    const { csrf_token } = (await (
      await page.request.get("/api/v1/auth/session")
    ).json()) as { csrf_token: string };
    const headers = { "X-CSRF-Token": csrf_token, Origin: baseURL! };
    if (access === "editor") {
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
          await page.request.post(
            "/api/v1/servers/survival/integrations/discord",
            {
              headers,
              data: {
                enabled: true,
                channel_id: "400",
                subscriptions: [],
                card: { enabled: false, fields: [] },
              },
            },
          )
        ).status(),
      ).toBe(201);
    }
    const roleResponse = await page.request.post("/api/v1/roles", {
      headers,
      data: {
        name: "Server viewer",
        scope: "server",
        permissions:
          access === "editor" ? ["server.configure"] : ["server.view"],
      },
    });
    expect(roleResponse.ok()).toBeTruthy();
    const role = (await roleResponse.json()) as { id: number };
    expect(
      (
        await page.request.post("/api/v1/servers/survival/access/grants", {
          headers,
          data: { username: "viewer", access_role_id: role.id },
        })
      ).ok(),
    ).toBeTruthy();
    await page.getByRole("button", { name: "Sign out", exact: true }).click();
    await page.getByLabel("Username", { exact: true }).fill("viewer");
    await page
      .getByLabel("Password", { exact: true })
      .fill("campfire-e2e-password");
    await page.getByRole("button", { name: "Sign in", exact: true }).click();
    await page.goto("/#servers/survival/console");
    await expect(
      page.getByRole("heading", { name: "Survival", exact: true, level: 1 }),
    ).toBeVisible();
    if (access === "editor") {
      const drawer = await openServerIntegrations(page);
      await expect(
        drawer.getByRole("button", { name: "Add integration", exact: true }),
      ).toBeVisible();
      await drawer
        .getByRole("button", {
          name: "Edit integration for #server-updates",
          exact: true,
        })
        .click();
      await drawer
        .getByRole("switch", { name: "Enabled", exact: true })
        .uncheck();
      await saveServerIntegration(drawer);
      const result = (await (
        await page.request.get("/api/v1/servers/survival/integrations/discord")
      ).json()) as { items: { enabled: boolean }[] };
      expect(result.items[0]!.enabled).toBe(false);
      expect(
        (
          await page.request.get(
            "/api/v1/servers/survival/integrations/discord",
          )
        ).status(),
      ).toBe(200);
      expect(
        (await page.request.get("/api/v1/integrations/discord")).status(),
      ).toBe(403);
      return;
    }
    const actions = page.getByRole("button", {
      name: "More server actions",
      exact: true,
    });
    if (await actions.count()) await actions.click();
    await expect(
      page.getByRole("menuitem", { name: "Edit server", exact: true }),
    ).toHaveCount(0);
    expect(
      (
        await page.request.get("/api/v1/servers/survival/integrations/discord")
      ).status(),
    ).toBe(403);
  });
}

test("administrators add, edit, test, disable, and remove Discord connections", async ({
  page,
}) => {
  await login(page);
  const open = async () => {
    await page
      .getByRole("button", {
        name: "Account options for Developer",
        exact: true,
      })
      .click();
    await page
      .getByRole("button", { name: "Integrations", exact: true })
      .click();
    await expect(page.getByText("Loading…", { exact: true })).toHaveCount(0);
  };
  await open();
  const drawer = page.getByRole("dialog", {
    name: "Integrations",
    exact: true,
  });
  const token = drawer.getByLabel("Bot token", { exact: true });
  await expect(token).toHaveCount(0);
  await expect(drawer.getByRole("switch")).toHaveCount(0);
  await drawer
    .getByRole("button", { name: "Add integration", exact: true })
    .click();
  await drawer
    .getByRole("combobox", { name: "Integration", exact: true })
    .click();
  await page.getByRole("option", { name: "Discord", exact: true }).click();
  const save = drawer.getByRole("button", {
    name: "Add integration",
    exact: true,
  });
  await expect(save).toBeDisabled();
  await token.fill("campfire-test-discord-token");
  await expect(
    drawer.getByRole("combobox", { name: "Discord server", exact: true }),
  ).toHaveText("Campfire Community");
  await expect(save).toBeEnabled();
  const invite = drawer.getByRole("link", {
    name: "Add bot to Discord",
    exact: true,
  });
  await expect(invite).toHaveAttribute(
    "href",
    /^https:\/\/discord\.com\/oauth2\/authorize\?/,
  );
  const inviteURL = new URL((await invite.getAttribute("href"))!);
  expect(inviteURL.searchParams.get("scope")).toBe("bot");
  expect(inviteURL.searchParams.get("permissions")).toBe("84992");
  expect(inviteURL.href).not.toContain("campfire-test-discord-token");
  await screenshot(page, "integrations-discord-add.png", drawer);
  await save.click();
  const connection = drawer.getByRole("table", {
    name: "Connected integrations",
    exact: true,
  });
  await expect(connection).toContainText("Campfire Community");
  await expect(token).toHaveCount(0);
  await screenshot(page, "integrations.png", drawer);
  await page.setViewportSize({ width: 320, height: 844 });
  await expect(connection.getByRole("switch")).toBeInViewport();
  await connection
    .getByRole("cell")
    .filter({ hasText: "Campfire Community" })
    .click();
  await page.setViewportSize({ width: 1400, height: 900 });
  await expect(token).toHaveCount(0);
  await expect(
    drawer.getByRole("combobox", { name: "Discord server", exact: true }),
  ).toHaveText("Campfire Community");
  await drawer.getByText("Test delivery", { exact: true }).click();
  await expect(
    drawer.getByRole("combobox", { name: "Test channel", exact: true }),
  ).toBeEnabled();
  await screenshot(page, "integrations-discord.png", drawer);

  const channel = drawer.getByRole("combobox", {
    name: "Test channel",
    exact: true,
  });
  const send = drawer.getByRole("button", {
    name: "Send test message",
    exact: true,
  });
  await channel.click();
  await page
    .getByRole("option", {
      name: "#read-only (missing permissions)",
      exact: true,
    })
    .click();
  await expect(drawer).toContainText("Missing bot permissions: Send Messages.");
  await expect(send).toBeDisabled();
  await channel.click();
  await page
    .getByRole("option", { name: "#server-updates", exact: true })
    .click();
  await send.click();
  await expect(drawer).toContainText("Test message sent.");
  await page.keyboard.press("Escape");

  await page.reload();
  await open();
  await drawer
    .getByRole("button", { name: "Edit Discord integration", exact: true })
    .click();
  await expect(token).toHaveCount(0);
  await drawer
    .getByRole("button", { name: "Change bot token", exact: true })
    .click();
  await token.fill("invalid-token");
  await expect(drawer.getByRole("alert")).toContainText(
    "Discord rejected the bot token",
  );
  await drawer.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(token).toHaveCount(0);
  await expect(
    drawer.getByRole("combobox", { name: "Discord server", exact: true }),
  ).toHaveText("Campfire Community");
  await expect(
    drawer.getByRole("button", { name: "Save changes", exact: true }),
  ).toBeDisabled();
  await drawer.getByRole("button", { name: "Back", exact: true }).click();
  const enabled = drawer.getByRole("switch", {
    name: "Enable Discord connection",
    exact: true,
  });
  await enabled.uncheck();
  await expect(enabled).not.toBeChecked();
  await expect(enabled).toBeEnabled();
  await drawer
    .getByRole("button", { name: "Edit Discord integration", exact: true })
    .click();
  await drawer.getByText("Test delivery", { exact: true }).click();
  await expect(send).toBeDisabled();
  const remove = drawer.getByRole("button", {
    name: "Remove integration",
    exact: true,
  });
  await remove.click();
  const confirmation = page.getByRole("dialog", {
    name: "Remove Discord integration?",
    exact: true,
  });
  await expect(confirmation).toContainText(
    "Messages already sent to Discord will remain.",
  );
  await screenshot(page, "integration-remove.png", confirmation);
  await page.keyboard.press("Escape");
  await expect(confirmation).toHaveCount(0);
  await page.setViewportSize({ width: 320, height: 844 });
  await expect(remove).toBeInViewport({ ratio: 1 });
  await remove.click();
  await confirmation
    .getByRole("button", { name: "Remove integration", exact: true })
    .click();
  await expect(connection).toHaveCount(0);
  await expect(
    drawer.getByRole("button", { name: "Add integration", exact: true }),
  ).toBeEnabled();
  const status = (await (
    await page.request.get("/api/v1/integrations/discord")
  ).json()) as { configured: boolean };
  expect(status.configured).toBe(false);
  await page.keyboard.press("Escape");
  await expect(drawer).toHaveCount(0);
});

test("non-administrators cannot see integration setup or read its API", async ({
  page,
}) => {
  await login(page, "viewer");
  await page.getByRole("button", { name: /Account options for/ }).click();
  await expect(
    page.getByRole("button", { name: "Integrations", exact: true }),
  ).toHaveCount(0);
  expect(
    (await page.request.get("/api/v1/integrations/discord")).status(),
  ).toBe(403);
});
