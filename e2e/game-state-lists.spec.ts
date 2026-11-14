import { test, expect } from "./fixtures";
import { login, openScreen } from "./screens";
import type { GameStateView } from "../web/src/api/gameState";

test("current player lists render on server lists and clear when empty or stale", async ({
  page,
}) => {
  await login(page);
  let names = ["Alex", "Sam <guest>"];
  let fresh = true;
  await page.route("**/api/v1/monitoring", async (route) => {
    const response = await route.fetch();
    const snapshot = (await response.json()) as {
      collected_at: number;
      servers: { id: string; game?: GameStateView }[];
    };
    const server = snapshot.servers.find((item) => item.id === "survival");
    if (!server) throw new Error("Missing Survival fixture");
    server.game = {
      updated_at: snapshot.collected_at,
      fresh_for_seconds: 60,
      status: fresh ? "fresh" : "stale",
      ...(fresh
        ? {
            state: {
              players: names.length,
              fields: { player_names: names },
              field_labels: { player_names: "Online players" },
            },
          }
        : {}),
    };
    await route.fulfill({ response, json: snapshot });
  });
  await page.reload();
  const readings = page.getByRole("group", { name: "Game data", exact: true });
  await expect(readings).toContainText("Online players");
  await expect(readings).toContainText("Alex, Sam <guest>");
  await openScreen(page, "servers", "Servers");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(readings).toContainText("Alex, Sam <guest>");
  names = [];
  await page.reload();
  await expect(readings).toContainText("None");
  await expect(readings).not.toContainText("Alex");
  names = ["Taylor"];
  await page.reload();
  await expect(readings).toContainText("Taylor");
  fresh = false;
  await page.reload();
  await expect(readings).toHaveCount(0);
  await expect(page.getByText("Taylor", { exact: true })).toHaveCount(0);
});
