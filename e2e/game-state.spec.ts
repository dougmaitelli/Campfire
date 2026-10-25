import { test, expect } from "./fixtures";
import { login } from "./screens";
import type { GameStateView } from "../web/src/api/gameState";

interface GameMonitoringResponse {
  collected_at: number;
  servers: { id: string; game?: GameStateView; usage: { state: string } }[];
}

test("game state shows optional fields and hides stale values", async ({
  page,
}) => {
  await login(page);
  let mode: "full" | "players" | "stale" | "expired" | "stopped" = "full";
  await page.route("**/api/v1/monitoring", async (route) => {
    const response = await route.fetch();
    const snapshot = (await response.json()) as GameMonitoringResponse;
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
                  },
          }),
    };
    await route.fulfill({ response, json: snapshot });
  });
  await page.reload();
  const readings = page.locator(".game-state-readings");
  await expect(readings).toContainText("3 / 20");
  await expect(readings).toContainText("Integration world");
  await expect(readings).toContainText("Last updated");
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
});
