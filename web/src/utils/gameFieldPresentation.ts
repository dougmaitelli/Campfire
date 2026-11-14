import i18n from "../i18n";
import type en from "../i18n/locales/en.json";
import type { GameState } from "../api/gameState";
import type { GameFieldLabels } from "../api/gameFields";

export interface PresentedGameField {
  id: string;
  name: string;
  value: string;
  available: boolean;
}
type MessageKey = `gameState.${keyof typeof en.gameState}`;
export type TranslateGameField = (
  key: MessageKey,
  values: { players?: string; capacity?: string },
) => string;

interface BuiltinGameField {
  id: keyof Pick<GameState, "players" | "capacity">;
  labelKey: MessageKey;
  combineWith?: string;
  combinedValueKey?: MessageKey;
}

// Central frontend definitions. Keep behavior in parity with the Go gamefields
// package; integration-provided labels apply only to field.* custom IDs.
export const builtinGameFields: readonly BuiltinGameField[] = [
  {
    id: "players",
    labelKey: "gameState.players",
    combineWith: "capacity",
    combinedValueKey: "gameState.playersWithCapacity",
  },
  { id: "capacity", labelKey: "gameState.capacity" },
];

// Published cards and their previews explicitly use English until external
// integrations have a language preference of their own.
const englishGameField: TranslateGameField = (key, values = {}) =>
  i18n.t(key, { lng: "en", ...values });

export function gameStateFieldLabel(
  id: string,
  label?: string,
  translate: TranslateGameField = englishGameField,
): string {
  const builtin = builtinGameFields.find((field) => field.id === id);
  if (builtin) return translate(builtin.labelKey, {});
  return label ?? id.replace(/^field\./, "");
}

export function gameStateFieldIDs(state: GameState): string[] {
  return [
    ...builtinGameFields
      .filter((field) => typeof state[field.id] === "number")
      .map((field) => field.id),
    ...Object.keys(state.fields ?? {}).map((key) => `field.${key}`),
  ];
}

function valueText(
  state: GameState | undefined,
  id: string,
  translate: TranslateGameField,
  numbers: Intl.NumberFormat,
): string | undefined {
  if (!state) return undefined;
  const builtin = builtinGameFields.find((field) => field.id === id);
  const value = builtin
    ? state[builtin.id]
    : state.fields?.[id.replace(/^field\./, "")];
  if (Array.isArray(value)) {
    const items = value.flatMap((item) => {
      const text = scalarText(item, translate, numbers);
      return text === undefined ? [] : [text];
    });
    return items.length
      ? items.join(", ")
      : translate("gameState.emptyList", {});
  }
  return scalarText(value, translate, numbers);
}

function scalarText(
  value: unknown,
  translate: TranslateGameField,
  numbers: Intl.NumberFormat,
): string | undefined {
  if (typeof value === "boolean")
    return translate(value ? "gameState.yes" : "gameState.no", {});
  if (typeof value === "number")
    return Number.isFinite(value) ? numbers.format(value) : undefined;
  if (typeof value === "string" && value.trim()) return value;
  return undefined;
}

// Keep this adapter in parity with gamefields.Present. Only selected,
// available companions may be combined.
export function presentGameFields(
  state: GameState | undefined,
  ids: string[] = state ? gameStateFieldIDs(state) : [],
  translate: TranslateGameField = englishGameField,
  locale = "en",
): PresentedGameField[] {
  const numberOptions = { maximumSignificantDigits: 21 };
  let numbers: Intl.NumberFormat;
  try {
    numbers = new Intl.NumberFormat(locale, numberOptions);
  } catch {
    numbers = new Intl.NumberFormat("en", numberOptions);
  }
  const values = new Map(
    ids.flatMap((id) => {
      const value = valueText(state, id, translate, numbers);
      return value === undefined ? [] : [[id, value] as const];
    }),
  );
  const combined = new Map<string, string>();
  const omitted = new Set<string>();
  for (const definition of builtinGameFields) {
    if (
      definition.combineWith &&
      definition.combinedValueKey &&
      values.has(definition.id) &&
      values.has(definition.combineWith)
    ) {
      combined.set(
        definition.id,
        translate(definition.combinedValueKey, {
          [definition.id]: values.get(definition.id)!,
          [definition.combineWith]: values.get(definition.combineWith)!,
        }),
      );
      omitted.add(definition.combineWith);
    }
  }
  return ids
    .filter((id) => !omitted.has(id))
    .map((id) => ({
      id,
      name: gameStateFieldLabel(
        id,
        customGameFieldLabel(state?.field_labels, id),
        translate,
      ),
      value: combined.get(id) ?? values.get(id) ?? "",
      available: values.has(id),
    }));
}

export function customGameFieldLabel(
  labels: GameFieldLabels | undefined,
  id: string,
): string | undefined {
  if (!id.startsWith("field.")) return undefined;
  const key = id.slice(6);
  return labels && Object.hasOwn(labels, key) ? labels[key] : undefined;
}
