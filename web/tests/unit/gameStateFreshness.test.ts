import { expect, test } from "vitest";
import { gameStateFresh, isGameStateView } from "../../src/api/gameState";
import { GameLimits } from "../../src/config/policies";

test("game state uses Campfire's receipt timestamp and the shared UI freshness window", () => {
  const view = { updated_at: 1000, state: { players: 3 } };
  expect(isGameStateView(view)).toBe(true);
  expect(gameStateFresh(view, 1000)).toBe(true);
  expect(gameStateFresh(view, 1000 + GameLimits.StateFreshSeconds - 1)).toBe(
    true,
  );
  expect(gameStateFresh(view, 1000 + GameLimits.StateFreshSeconds)).toBe(false);
  expect(gameStateFresh(view, 999)).toBe(false);
  expect(gameStateFresh({ updated_at: 1000 }, 1001)).toBe(false);
});

test.each([-1, 0.5, NaN, "1000", null, undefined])(
  "rejects an invalid receive timestamp %s",
  (updated_at) => {
    expect(isGameStateView({ updated_at, state: { players: 3 } })).toBe(false);
  },
);
