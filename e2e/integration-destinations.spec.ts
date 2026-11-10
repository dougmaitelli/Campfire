import { test, expect, screenshot } from "./fixtures";
import { login } from "./screens";
import {
  openServerIntegrations,
  saveServerIntegration,
} from "./integration-screens";

test("integration drafts swap channels and recover a committed save with a lost response", async ({
  page,
  baseURL,
}) => {
  test.setTimeout(90_000);
  await login(page);
  const session = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const headers = { "X-CSRF-Token": session.csrf_token, Origin: baseURL! };
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
  const base = "/api/v1/servers/survival/integrations/discord";
  const created: string[] = [];
  for (const [channel, category] of [
    ["400", "backup"],
    ["402", "power"],
  ] as const) {
    const response = await page.request.post(base, {
      headers,
      data: {
        enabled: true,
        channel_id: channel,
        subscriptions: [{ category }],
        card: { enabled: false, fields: [] },
      },
    });
    expect(response.status()).toBe(201);
    created.push(((await response.json()) as { id: string }).id);
  }
  await page.goto("/#servers/survival/console");
  const drawer = await openServerIntegrations(page);
  const table = drawer.getByRole("table", {
    name: "Integrations",
    exact: true,
  });
  const back = drawer.getByRole("button", {
    name: "Back to integrations",
    exact: true,
  });
  for (const [contents, channel] of [
    ["Backups", "#other-updates"],
    ["Power events", "#server-updates"],
  ] as const) {
    await table.getByRole("cell", { name: contents, exact: true }).click();
    await drawer
      .getByRole("combobox", { name: "Discord channel", exact: true })
      .click();
    await page.getByRole("option", { name: channel, exact: true }).click();
    await back.click();
  }
  await saveServerIntegration(drawer);
  const swapped = (await (await page.request.get(base)).json()) as {
    items: { id: string; channel_id: string }[];
  };
  expect(swapped.items.find((item) => item.id === created[0])?.channel_id).toBe(
    "402",
  );
  expect(swapped.items.find((item) => item.id === created[1])?.channel_id).toBe(
    "400",
  );
  await openServerIntegrations(page);
  await table.getByRole("cell", { name: "Backups", exact: true }).click();
  await drawer
    .getByRole("button", { name: "Remove integration", exact: true })
    .click();
  await page
    .getByRole("dialog", { name: "Remove server integration?", exact: true })
    .getByRole("button", { name: "Remove integration", exact: true })
    .click();
  await drawer
    .getByRole("button", { name: "Add integration", exact: true })
    .click();
  await drawer
    .getByRole("combobox", { name: "Discord channel", exact: true })
    .click();
  await page
    .getByRole("option", { name: "#other-updates", exact: true })
    .click();
  await drawer.getByRole("switch", { name: "Enabled", exact: true }).check();
  await drawer
    .getByRole("checkbox", { name: "Show a live server card", exact: true })
    .uncheck();
  const batches: unknown[] = [];
  await page.route(`**${base}`, async (route) => {
    if (route.request().method() !== "PUT") return route.continue();
    batches.push(route.request().postDataJSON());
    const response = await route.fetch();
    expect(response.status()).toBe(200);
    if (batches.length === 1) await route.abort("connectionclosed");
    else await route.fulfill({ response });
  });
  await drawer.getByRole("button", { name: "Review", exact: true }).click();
  const save = drawer.getByRole("button", {
    name: "Save changes",
    exact: true,
  });
  await save.click();
  await expect(drawer.getByRole("alert")).toBeVisible();
  await expect(save).toBeEnabled();
  await save.click();
  await expect(drawer).toHaveCount(0);
  expect(batches).toHaveLength(2);
  expect(batches[1]).toEqual(batches[0]);
  const final = (await (await page.request.get(base)).json()) as {
    items: { id: string; channel_id: string }[];
  };
  expect(final.items).toHaveLength(2);
  expect(final.items.some((item) => item.id === created[0])).toBe(false);
  expect(final.items.find((item) => item.id === created[1])?.channel_id).toBe(
    "400",
  );
  expect(final.items.filter((item) => item.channel_id === "402")).toHaveLength(
    1,
  );
});

test("independent destinations separate backups, public cards, and scoped game rules", async ({
  page,
  baseURL,
}) => {
  test.setTimeout(90_000);
  await login(page);
  const session = (await (
    await page.request.get("/api/v1/auth/session")
  ).json()) as { csrf_token: string };
  const headers = { "X-CSRF-Token": session.csrf_token, Origin: baseURL! };
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
  const base = "/api/v1/servers/survival/integrations/discord";
  const publicResponse = await page.request.post(base, {
    headers,
    data: {
      enabled: true,
      channel_id: "400",
      subscriptions: [
        {
          category: "game",
          rules: [
            { type: "player.joined", fields: { player: "doug" } },
            { type: "game.announcement" },
          ],
        },
      ],
      card: { enabled: true, fields: [] },
    },
  });
  expect(publicResponse.status()).toBe(201);
  const publicBinding = (await publicResponse.json()) as { id: string };
  const adminResponse = await page.request.post(base, {
    headers,
    data: {
      enabled: true,
      channel_id: "402",
      subscriptions: [{ category: "backup" }],
      card: { enabled: false, fields: [] },
    },
  });
  expect(adminResponse.status()).toBe(201);
  const adminBinding = (await adminResponse.json()) as { id: string };
  for (const [id, category, type, title, data] of [
    ["announcement", "game", "game.announcement", "Public announcement", {}],
    ["doug", "game", "player.joined", "Doug joined", { player: "doug" }],
    ["alex", "game", "player.joined", "Alex joined", { player: "Alex" }],
    ["backup", "backup", "backup.succeeded", "Private backup completed", {}],
  ] as const) {
    expect(
      (
        await page.request.post("/__test/integrations/event", {
          headers: { "X-Campfire-E2E": "integration" },
          data: {
            id: `game:destinations-${id}`,
            server_id: "survival",
            category,
            type,
            title,
            data,
            occurred_at: 1792670400,
          },
        })
      ).ok(),
    ).toBeTruthy();
  }
  await expect
    .poll(async () => {
      const items = (await (
        await page.request.get(`${base}/${publicBinding.id}/messages`)
      ).json()) as { state: string }[];
      return items.filter((item) => item.state === "delivered").length;
    })
    .toBe(2);
  await expect
    .poll(async () => {
      const items = (await (
        await page.request.get(`${base}/${adminBinding.id}/messages`)
      ).json()) as { state: string }[];
      return items.filter((item) => item.state === "delivered").length;
    })
    .toBe(1);
  await page.goto("/mock/discord");
  await expect(
    page.getByRole("heading", { name: "Survival", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", {
      name: "Survival: Public announcement",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Survival: Doug joined", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Survival: Alex joined", { exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByText("Survival: Private backup completed", { exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", { name: "# other-updates", exact: true })
    .click();
  await expect(
    page.getByRole("heading", {
      name: "Survival: Private backup completed",
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.getByRole("article")).toHaveCount(1);

  await page.goto("/#servers/survival/console");
  const drawer = await openServerIntegrations(page);
  const destinations = drawer.getByRole("table", {
    name: "Integrations",
    exact: true,
  });
  await expect(destinations.locator("tbody tr")).toHaveCount(2);
  await expect(
    destinations.getByRole("button", {
      name: "Edit integration for #server-updates",
      exact: true,
    }),
  ).toBeVisible();
  await screenshot(page, "server-integration-destinations.png", drawer);
  await page.setViewportSize({ width: 320, height: 844 });
  await expect(
    destinations.getByRole("columnheader", { name: "Enabled", exact: true }),
  ).toBeInViewport();
  await destinations
    .getByRole("cell", { name: "Game events · Server card", exact: true })
    .click();
  await page.setViewportSize({ width: 1400, height: 900 });
  const playerRule = drawer.getByRole("group", {
    name: "Event type 1",
    exact: true,
  });
  const announcementRule = drawer.getByRole("group", {
    name: "Event type 2",
    exact: true,
  });
  await expect(
    playerRule.getByRole("combobox", { name: "Field value", exact: true }),
  ).toHaveText("doug");
  await expect(
    announcementRule.getByRole("combobox", {
      name: "Event field",
      exact: true,
    }),
  ).toHaveCount(0);
  await expect(
    announcementRule.getByRole("button", {
      name: "Add field filter",
      exact: true,
    }),
  ).toBeVisible();
  const save = drawer.getByRole("button", {
    name: "Next: Access",
    exact: true,
  });
  await drawer.getByRole("switch", { name: "Enabled", exact: true }).uncheck();
  await drawer
    .getByRole("button", { name: "Back to integrations", exact: true })
    .click();
  await drawer.getByRole("button", { name: "General", exact: true }).click();
  await drawer
    .getByRole("button", { name: "Integrations", exact: true })
    .click();
  await destinations
    .getByRole("button", {
      name: "Edit integration for #server-updates",
      exact: true,
    })
    .click();
  await expect(
    drawer.getByRole("switch", { name: "Enabled", exact: true }),
  ).not.toBeChecked();
  await drawer.getByRole("button", { name: "Cancel", exact: true }).click();
  expect(
    (
      (await (
        await page.request.get(`${base}/${publicBinding.id}`)
      ).json()) as { enabled: boolean }
    ).enabled,
  ).toBe(true);
  await openServerIntegrations(page);
  await destinations
    .getByRole("button", {
      name: "Edit integration for #server-updates",
      exact: true,
    })
    .click();
  await expect(
    drawer.getByRole("switch", { name: "Enabled", exact: true }),
  ).toBeChecked();
  await drawer
    .getByRole("button", { name: "Add event type", exact: true })
    .click();
  const unfinishedRule = drawer.getByRole("group", {
    name: "Event type 3",
    exact: true,
  });
  await expect(
    unfinishedRule.getByRole("combobox", { name: "Event type", exact: true }),
  ).toHaveText("Choose an event type");
  await expect(save).toBeDisabled();
  await unfinishedRule
    .getByRole("button", { name: "Add field filter", exact: true })
    .click();
  await unfinishedRule
    .getByRole("combobox", { name: "Event field", exact: true })
    .click();
  await page.getByRole("option", { name: "Player name", exact: true }).click();
  await expect(save).toBeDisabled();
  await drawer.getByRole("button", { name: "General", exact: true }).click();
  await drawer
    .getByRole("button", { name: "Integrations", exact: true })
    .click();
  await expect(
    unfinishedRule.getByRole("combobox", { name: "Event type", exact: true }),
  ).toHaveText("Choose an event type");
  await expect(
    unfinishedRule.getByRole("combobox", { name: "Event field", exact: true }),
  ).toHaveText("Player name");
  await expect(save).toBeDisabled();
  await unfinishedRule
    .getByRole("combobox", { name: "Event type", exact: true })
    .click();
  await page
    .getByRole("option", { name: "All event types", exact: true })
    .click();
  await expect(save).toBeEnabled();
  await saveServerIntegration(drawer);
  await openServerIntegrations(page);
  await destinations
    .getByRole("button", {
      name: "Edit integration for #server-updates",
      exact: true,
    })
    .click();
  const saved = (await (
    await page.request.get(`${base}/${publicBinding.id}`)
  ).json()) as {
    subscriptions: {
      category: string;
      rules?: { type: string; fields?: Record<string, string> }[];
    }[];
  };
  expect(
    saved.subscriptions.find((item) => item.category === "game")?.rules?.[2],
  ).toEqual({
    type: "",
    fields: { player: "" },
  });
  await unfinishedRule
    .getByRole("combobox", { name: "Event field", exact: true })
    .click();
  await page
    .getByRole("option", { name: "Select a field", exact: true })
    .click();
  await expect(save).toBeDisabled();
  await drawer
    .getByRole("button", { name: "Remove event type 3", exact: true })
    .click();
  await drawer
    .getByRole("button", { name: "Back to integrations", exact: true })
    .click();
  await drawer
    .getByRole("button", {
      name: "Edit integration for #other-updates",
      exact: true,
    })
    .click();
  await expect(
    drawer.getByRole("checkbox", { name: "Backups", exact: true }),
  ).toBeChecked();
  await expect(
    drawer.getByRole("checkbox", {
      name: "Show a live server card",
      exact: true,
    }),
  ).not.toBeChecked();
  await expect(
    drawer.getByRole("checkbox", { name: "players", exact: true }),
  ).toHaveCount(0);
  await drawer
    .getByRole("button", { name: "Remove integration", exact: true })
    .click();
  const confirmation = page.getByRole("dialog", {
    name: "Remove server integration?",
    exact: true,
  });
  await screenshot(page, "integration-destination-remove.png", confirmation);
  await confirmation
    .getByRole("button", { name: "Remove integration", exact: true })
    .click();
  await expect(destinations.locator("tbody tr")).toHaveCount(1);
  expect((await page.request.get(`${base}/${adminBinding.id}`)).status()).toBe(
    200,
  );
  await saveServerIntegration(drawer);
  expect((await page.request.get(`${base}/${adminBinding.id}`)).status()).toBe(
    404,
  );
  const remaining = (await (await page.request.get(base)).json()) as {
    items: { id: string }[];
  };
  expect(remaining.items.map((item) => item.id)).toEqual([publicBinding.id]);
  await openServerIntegrations(page);
  await destinations
    .getByRole("button", {
      name: "Edit integration for #server-updates",
      exact: true,
    })
    .click();
  await page.keyboard.press("Escape");
  await expect(drawer).toHaveCount(0);
  await openServerIntegrations(page);
  await destinations
    .getByRole("button", {
      name: "Edit integration for #server-updates",
      exact: true,
    })
    .click();
  await drawer.getByRole("button", { name: "Close", exact: true }).click();
  await expect(drawer).toHaveCount(0);
});
