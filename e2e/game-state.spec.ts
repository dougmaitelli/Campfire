import { test, expect, prepareScreenshot } from "./fixtures";
import { login, openScreen } from "./screens";
import type { GameStateView } from "../web/src/api/gameState";

interface GameMonitoringResponse {
  collected_at: number;
  servers: { id: string; game?: GameStateView; usage: { state: string } }[];
}

test("game state shows optional fields and hides stale values", async ({
  page,
}) => {
  await login(page);
  const collectedAt = await page.evaluate(() => Math.floor(Date.now() / 1000));
  let mode: "full" | "unlabeled" | "players" | "stale" | "expired" | "stopped" =
    "full";
  await page.route("**/api/v1/monitoring", async (route) => {
    const response = await route.fetch();
    const snapshot = (await response.json()) as GameMonitoringResponse;
    snapshot.collected_at = collectedAt;
    const server = snapshot.servers.find((item) => item.id === "survival");
    if (!server) throw new Error("Missing survival fixture");
    if (mode === "stopped") server.usage.state = "stopped";
    server.game = {
      updated_at: snapshot.collected_at - (mode === "expired" ? 60 : 0),
      fresh_for_seconds: 60,
      status: mode === "stale" ? "stale" : "fresh",
      ...(mode === "stale"
        ? {}
        : {
            state:
              mode === "players"
                ? { players: 0 }
                : {
                    players: 3,
                    capacity: 20,
                    fields: { map: "Integration world", game_mode: "Survival" },
                    ...(mode === "unlabeled"
                      ? {}
                      : {
                          field_labels: { map: "Map", game_mode: "Game mode" },
                        }),
                  },
          }),
    };
    await route.fulfill({ response, json: snapshot });
  });
  await page.reload();
  const readings = page.getByRole("group", { name: "Game data", exact: true });
  await expect(readings).toContainText("3 / 20");
  await expect(readings).toContainText("Integration world");
  await expect(readings).toContainText("Game mode");
  await expect(readings).not.toContainText("Last updated");
  await readings.hover();
  await expect(readings).toHaveAttribute("title", /^Last updated /);
  const serverRow = page.getByRole("row").filter({
    has: page.getByRole("button", { name: "Open Survival", exact: true }),
  });
  for (const [width, size] of [
    [1400, "desktop"],
    [1150, "narrow"],
  ] as const) {
    await page.setViewportSize({ width, height: 1000 });
    await prepareScreenshot(page);
    await expect(
      page.getByRole("table", {
        name: "Measured server resources",
        exact: true,
      }),
    ).toHaveScreenshot(["crops", `overview-game-state-table-${size}.png`]);
  }
  await page.setViewportSize({ width: 1000, height: 1000 });
  await prepareScreenshot(page);
  await expect(serverRow).toHaveScreenshot([
    "crops",
    "overview-game-state-row.png",
  ]);
  await page.setViewportSize({ width: 390, height: 844 });
  await prepareScreenshot(page);
  await serverRow.hover();
  await expect(serverRow).toHaveScreenshot([
    "crops",
    "overview-game-state-row-mobile.png",
  ]);
  await openScreen(page, "servers", "Servers");
  for (const [width, size] of [
    [1400, "desktop"],
    [1150, "narrow"],
  ] as const) {
    await page.setViewportSize({ width, height: 1000 });
    await prepareScreenshot(page);
    await page
      .getByRole("row")
      .filter({ has: readings })
      .filter({ visible: true })
      .hover();
    await expect(
      page.getByRole("table", { name: "Game servers", exact: true }),
    ).toHaveScreenshot(["crops", `servers-game-state-table-${size}.png`]);
  }
  await readings.click();
  await expect(page).toHaveURL(/#servers\/survival\/console$/);
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await openScreen(page, "servers", "Servers");
  await page.setViewportSize({ width: 1000, height: 1000 });
  await prepareScreenshot(page);
  await expect(
    page.getByRole("row").filter({
      has: page.getByRole("button", {
        name: "Open Survival",
        exact: true,
      }),
    }),
  ).toHaveScreenshot(["crops", "servers-game-state-row.png"]);
  await openScreen(page, "dashboard", "Overview");
  await page.setViewportSize({ width: 1400, height: 1000 });
  await readings.click();
  await expect(page).toHaveURL(/#servers\/survival\/console$/);
  await openScreen(page, "dashboard", "Overview");
  mode = "unlabeled";
  await page.reload();
  await expect(readings.locator("dt")).toHaveText([
    "Players",
    "map",
    "game_mode",
  ]);
  mode = "players";
  await page.reload();
  await expect(readings).toContainText("Players0");
  await expect(readings).not.toContainText("Map");
  await expect(readings).not.toContainText("Integration world");
  for (const unavailable of ["stale", "expired", "stopped"] as const) {
    mode = unavailable;
    await page.reload();
    await expect(readings).toContainText("Game data unavailable");
    await expect(readings).not.toContainText("Players");
    await expect(readings).not.toContainText("Integration world");
  }
  await readings.focus();
  await expect(readings).toBeFocused();
  await expect(readings).toHaveAccessibleDescription(/^Last updated /);
});

test("field labels and raw keys retain their supplied casing", async ({
  page,
}) => {
  await login(page);
  const labels = ["World name", "Server.status", "Server URL", "Invite code"];
  await page.route("**/api/v1/monitoring", async (route) => {
    const response = await route.fetch();
    const snapshot = (await response.json()) as GameMonitoringResponse;
    const server = snapshot.servers.find((item) => item.id === "survival")!;
    server.game = {
      updated_at: snapshot.collected_at,
      fresh_for_seconds: 60,
      status: "fresh",
      state: {
        fields: {
          ...Object.fromEntries(labels.map((_, i) => [`label${i}`, "value"])),
          raw_key: "value",
        },
        field_labels: Object.fromEntries(
          labels.map((label, i) => [`label${i}`, label]),
        ),
      },
    };
    await route.fulfill({ response, json: snapshot });
  });
  await page.reload();
  const readings = page.getByRole("group", { name: "Game data", exact: true });
  await expect(readings.locator("dt")).toHaveText([...labels, "raw_key"]);
  expect(
    await readings
      .locator("dt")
      .evaluateAll((elements) =>
        elements.map((element) => (element as HTMLElement).innerText),
      ),
  ).toEqual([...labels, "raw_key"]);
});
