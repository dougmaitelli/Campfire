import { expect, test } from "vitest";
import { gameFieldLabel, validGameFieldLabels } from "../../src/api/gameFields";
import { gameEventFieldLabel } from "../../src/api/gameEvents";
import { isGameStateView } from "../../src/api/gameState";

test("field labels are optional plain text with exact key fallback", () => {
  expect(gameFieldLabel({ game_mode: "Game mode" }, "game_mode")).toBe(
    "Game mode",
  );
  expect(gameFieldLabel(undefined, "game_mode")).toBe("game_mode");
  expect(gameFieldLabel({}, "constructor")).toBe("constructor");
  const fields = { game_mode: "survival" };
  expect(validGameFieldLabels(undefined, fields)).toBe(true);
  expect(validGameFieldLabels({ game_mode: "Game mode" }, fields)).toBe(true);
  for (const labels of [
    { missing: "Missing" },
    { game_mode: "" },
    { game_mode: " Game mode " },
    { game_mode: "Game\nmode" },
    { game_mode: "界".repeat(43) },
    { game_mode: 42 },
  ])
    expect(validGameFieldLabels(labels, fields)).toBe(false);
});

test("filter labels respect event type and fall back when all-types labels disagree", () => {
  const options = {
    types: ["player.joined", "player.left"],
    fields: { player: ["Alex"], state: ["ready"] },
    field_labels: {
      "player.joined": { player: "Joining player", state: "Status" },
      "player.left": { player: "Leaving player", state: "Status" },
    },
  };
  expect(gameEventFieldLabel(options, "player.joined", "player")).toBe(
    "Joining player",
  );
  expect(gameEventFieldLabel(options, "player.left", "player")).toBe(
    "Leaving player",
  );
  expect(gameEventFieldLabel(options, "", "player")).toBe("player");
  expect(gameEventFieldLabel(options, "", "state")).toBe("Status");
  expect(gameEventFieldLabel(options, "unknown", "player")).toBe("player");
  expect(gameEventFieldLabel(options, "constructor", "name")).toBe("name");
});

test("monitoring validates custom labels without requiring them from older publishers", () => {
  const base = { updated_at: 1, fresh_for_seconds: 60, status: "fresh" };
  const state = { fields: { game_mode: "survival" } };
  expect(isGameStateView({ ...base, state })).toBe(true);
  expect(
    isGameStateView({
      ...base,
      state: { ...state, field_labels: { game_mode: "Game mode" } },
    }),
  ).toBe(true);
  expect(
    isGameStateView({
      ...base,
      state: { ...state, field_labels: { missing: "Missing" } },
    }),
  ).toBe(false);
});
