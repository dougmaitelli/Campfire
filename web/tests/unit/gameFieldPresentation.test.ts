import { expect, test } from "vitest";
import {
  builtinGameFields,
  gameStateFieldIDs,
  gameStateFieldLabel,
  presentGameFields,
  type PresentedGameField,
} from "../../src/utils/gameFieldPresentation";
import type { GameState } from "../../src/api/gameState";
import i18n from "../../src/i18n";

const cases: {
  name: string;
  state?: GameState;
  ids: string[];
  fields: PresentedGameField[];
}[] = [
  {
    name: "selected built-ins combine without duplicate capacity",
    state: { players: 0, capacity: 20 },
    ids: ["capacity", "players"],
    fields: [
      { id: "players", name: "Players", value: "0 / 20", available: true },
    ],
  },
  {
    name: "unselected capacity never enters the value",
    state: { players: 3, capacity: 20 },
    ids: ["players"],
    fields: [{ id: "players", name: "Players", value: "3", available: true }],
  },
  {
    name: "capacity can be selected independently",
    state: { players: 3, capacity: 0 },
    ids: ["capacity"],
    fields: [{ id: "capacity", name: "Capacity", value: "0", available: true }],
  },
  {
    name: "missing players does not suppress capacity",
    state: { capacity: 20 },
    ids: ["players", "capacity"],
    fields: [
      { id: "players", name: "Players", value: "", available: false },
      { id: "capacity", name: "Capacity", value: "20", available: true },
    ],
  },
  {
    name: "missing state exposes no values",
    ids: ["players", "capacity", "field.world_name"],
    fields: [
      { id: "players", name: "Players", value: "", available: false },
      { id: "capacity", name: "Capacity", value: "", available: false },
      {
        id: "field.world_name",
        name: "world_name",
        value: "",
        available: false,
      },
    ],
  },
  {
    name: "labels, raw keys, and values retain their spelling",
    state: {
      fields: {
        world: "Meadows",
        raw_key: "unchanged",
        url: "https://example.test",
        pvp: false,
        round: 0,
        empty: "  ",
      },
      field_labels: { world: "World name", url: "Server URL", round: "Round" },
    },
    ids: [
      "field.world",
      "field.raw_key",
      "field.url",
      "field.pvp",
      "field.round",
      "field.empty",
      "field.missing",
    ],
    fields: [
      {
        id: "field.world",
        name: "World name",
        value: "Meadows",
        available: true,
      },
      {
        id: "field.raw_key",
        name: "raw_key",
        value: "unchanged",
        available: true,
      },
      {
        id: "field.url",
        name: "Server URL",
        value: "https://example.test",
        available: true,
      },
      { id: "field.pvp", name: "pvp", value: "No", available: true },
      { id: "field.round", name: "Round", value: "0", available: true },
      { id: "field.empty", name: "empty", value: "", available: false },
      { id: "field.missing", name: "missing", value: "", available: false },
    ],
  },
  {
    name: "custom fields cannot override built-ins or trigger metric formatting",
    state: {
      players: 3,
      capacity: 20,
      fields: { players: true, capacity: true },
      field_labels: { players: "Player tracking", capacity: "Has slots" },
    },
    ids: ["players", "capacity", "field.players", "field.capacity"],
    fields: [
      { id: "players", name: "Players", value: "3 / 20", available: true },
      {
        id: "field.players",
        name: "Player tracking",
        value: "Yes",
        available: true,
      },
      {
        id: "field.capacity",
        name: "Has slots",
        value: "Yes",
        available: true,
      },
    ],
  },
];

test.each(cases)("field presentation: $name", ({ state, ids, fields }) => {
  expect(presentGameFields(state, ids)).toEqual(fields);
});

test("built-in definitions have translations and unambiguous IDs", () => {
  expect(new Set(builtinGameFields.map((field) => field.id)).size).toBe(
    builtinGameFields.length,
  );
  for (const definition of builtinGameFields)
    expect(i18n.t(definition.labelKey, { lng: "en" })).not.toBe(
      definition.labelKey,
    );
});

test("translations apply to built-ins and combined values, never publisher labels", () => {
  const messages: Record<string, string> = {
    "gameState.players": "Connected players",
    "gameState.capacity": "Player limit",
    "gameState.playersWithCapacity": "{{players}} of {{capacity}}",
  };
  const translate = (key: string, values: Record<string, string> = {}) => {
    let text = messages[key]!;
    for (const [key, value] of Object.entries(values))
      text = text.replaceAll(`{{${key}}}`, value);
    return text;
  };
  const fields = presentGameFields(
    {
      players: 3,
      capacity: 20,
      fields: { world: "Meadows" },
      field_labels: { world: "World name" },
    },
    undefined,
    translate,
  );
  expect(fields.map(({ name, value }) => [name, value])).toEqual([
    ["Connected players", "3 of 20"],
    ["World name", "Meadows"],
  ]);
  expect(gameStateFieldLabel("capacity", undefined, translate)).toBe(
    "Player limit",
  );
});

test("i18n resolves labels and interpolated values", () => {
  const fields = presentGameFields(
    { players: 3, capacity: 20 },
    undefined,
    i18n.t,
  );
  expect(fields[0]?.name).toBe("Players");
  expect(fields[0]?.value).toBe("3 / 20");
});

test("numeric formatting follows the UI locale without losing precision", () => {
  const state = {
    players: 12345,
    capacity: 20000,
    fields: { fraction: 12.5, small: 0.0000001, precise: 1.23456789012345 },
  };
  expect(presentGameFields(state).map((field) => field.value)).toEqual([
    "12,345 / 20,000",
    "12.5",
    "0.0000001",
    "1.23456789012345",
  ]);
  expect(
    presentGameFields(state, undefined, i18n.t, "de-DE").map(
      (field) => field.value,
    ),
  ).toEqual(["12.345 / 20.000", "12,5", "0,0000001", "1,23456789012345"]);
});

test("display labels remain verbatim", () => {
  for (const label of [
    "World name",
    "Server.status",
    "Server URL",
    "Invite code",
  ])
    expect(gameStateFieldLabel("field.world", label)).toBe(label);
});

test("discovery distinguishes custom and built-in fields and preserves raw keys", () => {
  expect(
    gameStateFieldIDs({
      players: 0,
      fields: { players: "Alex", world: "Meadows" },
    }),
  ).toEqual(["players", "field.players", "field.world"]);
  expect(gameStateFieldLabel("field.constructor")).toBe("constructor");
  expect(gameStateFieldLabel("field.world_name")).toBe("world_name");
});
