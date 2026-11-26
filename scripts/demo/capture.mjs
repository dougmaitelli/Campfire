// Capture real UI states from the disposable E2E harness, never production.
// Run after building web/dist and starting backend/e2e on 127.0.0.1:8099.
import {
  chromium,
  expect,
} from "../../e2e/node_modules/@playwright/test/index.mjs";
import { mkdir } from "node:fs/promises";
import path from "node:path";

const baseURL = "http://127.0.0.1:8099";
const output = path.join(import.meta.dirname, "assets");
await mkdir(output, { recursive: true });
const browser = await chromium.launch({ headless: true, channel: "chrome" });
const page = await browser.newPage({
  baseURL,
  viewport: { width: 1400, height: 1000 },
  deviceScaleFactor: 1,
  timezoneId: "UTC",
});
await page.clock.setFixedTime(new Date("2026-10-22T12:00:00Z"));
async function capture(name) {
  await page.evaluate(() => globalThis.document.fonts.ready);
  await page.mouse.move(0, 0);
  await page.waitForTimeout(300);
  await expect(
    page.getByText("Connection to Campfire interrupted. Retrying…", {
      exact: true,
    }),
  ).toHaveCount(0);
  await page.screenshot({
    path: path.join(output, name + ".png"),
    animations: "disabled",
  });
  console.log("Captured", name);
}
async function choose(scope, label, option) {
  await scope.getByRole("combobox", { name: label, exact: true }).click();
  await page
    .getByRole("option", { name: option, exact: typeof option === "string" })
    .click();
}
async function top(dialog) {
  await dialog.getByRole("heading").first().scrollIntoViewIfNeeded();
  // Headers are sticky; scroll the actual content container back to its start.
  await dialog.evaluate((el) => {
    for (const child of el.querySelectorAll("*")) {
      if (
        child.scrollHeight > child.clientHeight &&
        ["auto", "scroll"].includes(
          globalThis.getComputedStyle(child).overflowY,
        )
      )
        child.scrollTop = 0;
    }
  });
}
try {
  const reset = await page.request.post("/__test/reset", {
    headers: { "X-Campfire-E2E": "reset" },
  });
  if (reset.status() !== 204) throw new Error("Not the disposable E2E harness");
  await page.goto("/");
  await page.getByLabel("Username", { exact: true }).fill("admin");
  await page
    .getByLabel("Password", { exact: true })
    .fill("Campfire-E2E-Password1!");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await expect(
    page.getByRole("navigation", { name: "Main navigation" }),
  ).toBeVisible();

  await page.goto("/#servers");
  await page
    .getByRole("button", { name: "+ Create server", exact: true })
    .click();
  const create = page.getByRole("dialog", {
    name: "Create server",
    exact: true,
  });
  await create
    .getByRole("textbox", { name: "Server name", exact: true })
    .fill("Campfire Survival");
  await choose(create, "Spark", "Europe");
  await top(create);
  await capture("create-general");
  await create
    .getByRole("button", { name: "Next: Variables", exact: true })
    .click();
  await capture("create-variables");
  for (const label of [
    "Next: Resources & ports",
    "Next: Backups",
    "Next: Access",
    "Next: Review",
  ]) {
    await create.getByRole("button", { name: label, exact: true }).click();
  }
  await top(create);
  await capture("create-review");
  await create
    .getByRole("button", { name: "Create server", exact: true })
    .click();
  await expect(create).toHaveCount(0);

  await page.goto("/#servers/creative/console");
  await page
    .getByRole("button", { name: "More server actions", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Transfer", exact: true }).click();
  const transfer = page.getByRole("dialog", {
    name: "Transfer Creative",
    exact: true,
  });
  await choose(transfer, "Destination Spark", "America");
  await capture("transfer-destination");
  await transfer
    .getByRole("button", { name: "Close", exact: true })
    .last()
    .click();
  await expect(transfer).toHaveCount(0);
  // The in-progress transfer uses the same deterministic data as jobFixtures.ts;
  // it is not a throughput measurement or an instant-completion claim.
  const job = {
    id: "demo-transfer",
    type: "transfer",
    status: "running",
    step: "copying",
    source_server_id: "creative",
    target_server_id: "creative",
    source_spark_id: "mock-europe",
    target_spark_id: "mock-america",
    source_name: "Creative",
    target_name: "Creative",
    source_spark_name: "Europe",
    target_spark_name: "America",
    transferred_bytes: 384 * 1024 ** 2,
    total_bytes: 1024 ** 3,
    error: "",
    created_at: 1792670400,
    updated_at: 1792670400,
    finished_at: null,
    can_retry: false,
    can_cancel: true,
    waiting_for_sparks: [],
  };
  const completed = {
    ...job,
    id: "previous-transfer",
    status: "completed",
    source_spark_name: "America",
    target_spark_name: "Europe",
    source_spark_id: "mock-america",
    target_spark_id: "mock-europe",
    transferred_bytes: job.total_bytes,
    can_cancel: false,
    created_at: 1792584000,
    updated_at: 1792584300,
    finished_at: 1792584300,
  };
  const jobs = [job, completed];
  const entry = (item) => ({
    key: `job:${item.id}`,
    kind: "job",
    title: "transfer",
    state: item.status,
    created_at: item.created_at,
    source_available: false,
    notification_count: 0,
  });
  await page.route("**/api/v1/servers/creative/activity?*", (route) =>
    route.fulfill({ json: { items: jobs.map(entry), next: "" } }),
  );
  await page.route("**/api/v1/servers/creative/activity/job%3A*", (route) => {
    const id = decodeURIComponent(
      new URL(route.request().url()).pathname.split("/").at(-1),
    ).slice(4);
    const item = jobs.find((item) => item.id === id);
    return route.fulfill({
      json: { ...entry(item), job: item, can_retry_operation: false },
    });
  });
  await page.goto("/#servers/creative/activity?kind=operations");
  await expect(
    page
      .getByRole("region", { name: "Activity", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(2);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await capture("transfer-history");

  const session = await (await page.request.get("/api/v1/auth/session")).json();
  const headers = { "X-CSRF-Token": session.csrf_token, Origin: baseURL };
  const connection = await page.request.put("/api/v1/integrations/discord", {
    headers,
    data: {
      enabled: true,
      guild_id: "200",
      token: "campfire-test-discord-token",
    },
  });
  if (!connection.ok()) throw new Error(await connection.text());
  const binding = await page.request.post(
    "/api/v1/servers/survival/integrations/discord",
    {
      headers,
      data: {
        enabled: true,
        channel_id: "400",
        subscriptions: [
          {
            category: "game",
            rules: [
              { type: "player.joined" },
              { type: "minecraft.advancement" },
            ],
          },
          { category: "power" },
          { category: "backup" },
        ],
        card: {
          enabled: true,
          fields: ["players", "capacity", "field.invite_code"],
        },
      },
    },
  );
  if (!binding.ok()) throw new Error(await binding.text());
  const saved = await binding.json();
  await expect
    .poll(
      async () => {
        const current = await (
          await page.request.get(
            `/api/v1/servers/survival/integrations/discord/${saved.id}`,
          )
        ).json();
        return current.card_status.state;
      },
      { timeout: 20000 },
    )
    .toBe("published");
  // Match the deterministic fresh-game-state fixture in e2e/integrations.spec.ts.
  // The seed's October state is expired relative to the backend's real clock.
  await page.route("**/api/v1/monitoring", async (route) => {
    const response = await route.fetch();
    const snapshot = await response.json();
    snapshot.servers.find((server) => server.id === "survival").game = {
      updated_at: snapshot.collected_at,
      fresh_for_seconds: 60,
      status: "fresh",
      state: {
        players: 3,
        capacity: 20,
        fields: { invite_code: "test-invite" },
        field_labels: { invite_code: "Join code" },
      },
    };
    await route.fulfill({ response, json: snapshot });
  });
  await page.route(
    "**/api/v1/servers/survival/integrations/discord/card?*",
    async (route) => {
      const response = await route.fetch();
      const card = await response.json();
      card.state = "running";
      card.expires_at = 1792670460;
      card.addresses = ["games.example.test:25565 / TCP"];
      const values = {
        players: "3",
        capacity: "20",
        "field.invite_code": "test-invite",
      };
      for (const field of card.fields) {
        if (field.id === "field.invite_code") field.name = "Join code";
        field.value = values[field.id] ?? "";
        field.available = field.id in values;
      }
      card.preview.embeds[0].fields = [
        { name: "Status", value: "Running" },
        { name: "Address", value: card.addresses.join("\n") },
        ...card.fields.map((field) => ({
          name: field.name,
          value: field.value,
        })),
      ];
      await route.fulfill({ response, json: card });
    },
  );
  await page.goto("/#servers/survival/console");
  await page
    .getByRole("button", { name: "More server actions", exact: true })
    .click();
  await page
    .getByRole("menuitem", { name: "Edit server", exact: true })
    .click();
  const edit = page.getByRole("dialog", { name: "Edit server", exact: true });
  await edit.getByRole("button", { name: "Integrations", exact: true }).click();
  await edit
    .getByRole("button", { name: /^Edit integration for #/ })
    .first()
    .click();
  await edit
    .getByRole("checkbox", { name: "Game events", exact: true })
    .scrollIntoViewIfNeeded();
  await capture("publishing-settings");
  await edit
    .getByRole("region", { name: "Server card preview", exact: true })
    .scrollIntoViewIfNeeded();
  await expect(
    edit.getByRole("region", { name: "Server card preview", exact: true }),
  ).toContainText("Survival");
  await capture("server-card-preview");
  await edit.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(edit).toHaveCount(0);

  // Publish a real fixture event through the real queue to two mock destinations.
  const secondary = await page.request.post(
    "/api/v1/servers/survival/integrations/discord",
    {
      headers,
      data: {
        enabled: true,
        channel_id: "402",
        subscriptions: [{ category: "game" }],
        card: { enabled: false, fields: [] },
      },
    },
  );
  if (!secondary.ok()) throw new Error(await secondary.text());
  const publication = await page.request.post(
    "/__test/integrations/game-event",
    {
      headers: { "X-Campfire-E2E": "integration" },
      data: {
        server_id: "survival",
        event: {
          id: "demo-activity-linked",
          type: "player.joined",
          title: "Morgan joined the game",
          occurred_at: "2026-10-22T12:00:00Z",
          data: { player: "Morgan" },
        },
      },
    },
  );
  if (!publication.ok()) throw new Error(await publication.text());
  await page.goto("/#servers/survival/activity?kind=game");
  const activity = page.getByRole("region", { name: "Activity", exact: true });
  await expect(
    activity.getByRole("button", {
      name: "Morgan joined the game",
      exact: true,
    }),
  ).toBeVisible();
  await capture("game-activity");
  await activity
    .getByRole("button", { name: "Morgan joined the game", exact: true })
    .click();
  const notifications = page
    .getByRole("dialog")
    .getByRole("table", { name: "Notifications", exact: true });
  await expect(notifications.locator("tbody tr")).toHaveCount(2);
  await expect(
    notifications.getByText("Delivered", { exact: true }),
  ).toHaveCount(2, { timeout: 20000 });
  await capture("game-event");
} catch (error) {
  console.error(await page.locator("body").innerText());
  await page.screenshot({ path: path.join(output, "capture-error.png") });
  throw error;
} finally {
  await browser.close();
}
