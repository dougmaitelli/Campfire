import { expect, test } from "vitest";
import { isGameStateView } from "../../src/api/gameState";
import { isGameEvent } from "../../src/api/gameEvents";
import { GameLimits } from "../../src/config/policies";
import {
  gameStateFieldIDs,
  presentGameFields,
} from "../../src/utils/gameFieldPresentation";

const base = { updated_at: 1, fresh_for_seconds: 60, status: "fresh" };

test.each([
  ["names", ["Alex", "Sam"], true],
  ["empty", [], true],
  ["mixed scalars", ["Alex", 0, false], true],
  ["maximum items", Array(GameLimits.FieldListItems).fill("Alex"), true],
  ["too many items", Array(GameLimits.FieldListItems + 1).fill("Alex"), false],
  ["maximum string bytes", ["a".repeat(1024)], true],
  ["too many UTF-8 bytes", ["界".repeat(342)], false],
  ["nested list", [["Alex"]], false],
  ["object", [{ name: "Alex" }], false],
  ["null item", [null], false],
  ["null list", null, false],
  ["infinity", [Infinity], false],
  ["NaN", [NaN], false],
])("state list validation: %s", (_, value, valid) => {
  expect(
    isGameStateView({
      ...base,
      state: {
        fields: { player_names: value },
        field_labels: { player_names: "Online players" },
      },
    }),
  ).toBe(valid);
});

test("list fields preserve order, labels, scalar formatting, and empty versus missing", () => {
  const state = {
    fields: {
      player_names: ["Alex", "Sam", "Alex"],
      mixed: ["Round", 1234, true, false, "  "],
      empty: [],
      blank: ["", " "],
    },
    field_labels: { player_names: "Online players" },
  };
  const ids = [
    "field.player_names",
    "field.mixed",
    "field.empty",
    "field.blank",
    "field.missing",
  ];
  expect(gameStateFieldIDs(state)).toEqual(ids.slice(0, -1));
  expect(presentGameFields(state, ids)).toEqual([
    {
      id: ids[0],
      name: "Online players",
      value: "Alex, Sam, Alex",
      available: true,
    },
    {
      id: ids[1],
      name: "mixed",
      value: "Round, 1,234, Yes, No",
      available: true,
    },
    { id: ids[2], name: "empty", value: "None", available: true },
    { id: ids[3], name: "blank", value: "None", available: true },
    { id: ids[4], name: "missing", value: "", available: false },
  ]);
  expect(presentGameFields(undefined, ids.slice(0, 1))[0]).toMatchObject({
    available: false,
    value: "",
  });
  expect(
    presentGameFields(state, ["field.empty"], (key) =>
      key === "gameState.emptyList" ? "Empty list" : key,
    )[0]?.value,
  ).toBe("Empty list");
});

test("event data remains scalar-only", () => {
  const event = {
    id: 1,
    server_id: "server",
    spark_id: "spark",
    event_id: "one",
    type: "player.joined",
    title: "Joined",
    occurred_at: "2026-10-24T00:00:00Z",
    received_at: 1,
  };
  expect(isGameEvent({ ...event, data: { player: "Alex" } }, "server")).toBe(
    true,
  );
  expect(isGameEvent({ ...event, data: { player: ["Alex"] } }, "server")).toBe(
    false,
  );
});
