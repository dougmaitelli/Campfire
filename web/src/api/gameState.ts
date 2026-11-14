import { GameLimits } from "../config/policies";
import { GameStateStatus } from "../contracts/domains";
import { isRecord as object, isEnumValue } from "../utils/validation";
import { validGameFieldLabels, type GameFieldLabels } from "./gameFields";
export type GameStateScalar = string | number | boolean;
export type GameStateFieldValue = GameStateScalar | GameStateScalar[];

export interface GameState {
  players?: number;
  capacity?: number;
  fields?: Record<string, GameStateFieldValue>;
  field_labels?: GameFieldLabels;
}

export interface GameStateView {
  state?: GameState;
  updated_at: number;
  fresh_for_seconds: number;
  status: GameStateStatus;
}

function count(value: unknown): value is number {
  return (
    typeof value === "number" &&
    Number.isSafeInteger(value) &&
    value >= 0 &&
    value <= GameLimits.Players
  );
}

function scalar(value: unknown): value is GameStateScalar {
  return (
    typeof value === "boolean" ||
    (typeof value === "string" &&
      new TextEncoder().encode(value).length <= GameLimits.FieldStringLength) ||
    (typeof value === "number" && Number.isFinite(value))
  );
}

function fieldValue(value: unknown): value is GameStateFieldValue {
  return Array.isArray(value)
    ? value.length <= GameLimits.FieldListItems && value.every(scalar)
    : scalar(value);
}

export function isGameStateView(value: unknown): value is GameStateView {
  if (
    !object(value) ||
    !Number.isSafeInteger(value.updated_at) ||
    (value.updated_at as number) < 0 ||
    !Number.isSafeInteger(value.fresh_for_seconds) ||
    (value.fresh_for_seconds as number) <= 0 ||
    !isEnumValue(GameStateStatus, value.status)
  )
    return false;
  if (value.state === undefined) return value.status !== "fresh";
  if (value.status !== "fresh" || !object(value.state)) return false;
  const state = value.state;
  return (
    (state.players === undefined || count(state.players)) &&
    (state.capacity === undefined || count(state.capacity)) &&
    validGameFieldLabels(
      state.field_labels,
      object(state.fields) ? state.fields : {},
    ) &&
    (state.fields === undefined ||
      (object(state.fields) &&
        Object.entries(state.fields).length <= GameLimits.Fields &&
        Object.entries(state.fields).every(
          ([key, field]) =>
            /^[a-z][a-z0-9_.-]{0,63}$/.test(key) && fieldValue(field),
        )))
  );
}

export function gameStateFresh(view: GameStateView, now: number): boolean {
  return (
    view.status === GameStateStatus.Fresh &&
    view.state !== undefined &&
    now >= view.updated_at &&
    now - view.updated_at < view.fresh_for_seconds
  );
}
