import { GameLimits } from "../config/policies";
import { isRecord } from "../utils/validation";

export type GameFieldLabels = Record<string, string>;

export function gameFieldLabel(
  labels: GameFieldLabels | undefined,
  key: string,
): string {
  return labels && Object.hasOwn(labels, key) ? labels[key]! : key;
}

export function isGameFieldLabel(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.length > 0 &&
    new TextEncoder().encode(value).length <= GameLimits.FieldLabelLength &&
    value.trim() === value &&
    !/\p{Cc}/u.test(value)
  );
}

export function validGameFieldLabels(
  value: unknown,
  fields: Record<string, unknown>,
): boolean {
  return (
    value === undefined ||
    (isRecord(value) &&
      Object.keys(value).length <= GameLimits.Fields &&
      Object.entries(value).every(
        ([key, label]) =>
          Object.hasOwn(fields, key) &&
          /^[a-z][a-z0-9_.-]{0,63}$/.test(key) &&
          isGameFieldLabel(label),
      ))
  );
}
