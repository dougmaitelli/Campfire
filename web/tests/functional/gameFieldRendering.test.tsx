import { render, screen } from "@testing-library/react";
import { expect, test } from "vitest";
import i18n from "../../src/i18n";
import { GameStateReadings } from "../../src/components/servers/GameStateReadings";
import { RuntimeState } from "../../src/contracts/domains";
import type { ResourceUsage } from "../../src/api/monitoring";

const usage: ResourceUsage = {
  server_id: "server",
  state: RuntimeState.Running,
  cpu_percent: 0,
  memory_bytes: 0,
  disk_bytes: 0,
  sampled_at: 100,
  disk_sampled_at: 100,
  sample_age_seconds: 0,
  disk_age_seconds: 0,
  container_error: "",
  disk_error: "",
  disk_quota_enforced: false,
  disk_quota_error: "",
};

test("server readings preserve translated labels, display labels, and raw-key fallbacks", async () => {
  i18n.addResourceBundle("test", "translation", {
    gameState: {
      players: "Connected players",
      capacity: "Player limit",
      playersWithCapacity: "{{players}} of {{capacity}}",
      no: "No",
      yes: "Yes",
    },
  });
  await i18n.changeLanguage("test");
  const { container } = render(
    <GameStateReadings
      now={100}
      elapsed={0}
      usage={usage}
      game={{
        updated_at: 100,
        fresh_for_seconds: 60,
        status: "fresh",
        state: {
          players: 0,
          capacity: 20,
          fields: { world: "Meadows", raw_key: false, players: true },
          field_labels: { world: "World name", players: "Player tracking" },
        },
      }}
    />,
  );
  expect(
    [...container.querySelectorAll("dt")].map((element) => element.textContent),
  ).toEqual(["Connected players", "World name", "raw_key", "Player tracking"]);
  expect(screen.getByText("0 of 20")).toBeTruthy();
  expect(screen.queryByText("Player limit")).toBeNull();
  expect(screen.getByText("No")).toBeTruthy();
  expect(screen.getByText("Yes")).toBeTruthy();
});
