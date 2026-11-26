import { GameLimits } from "../config/policies";
import { isRecord as object } from "../utils/validation";
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
    (value.updated_at as number) < 0
  )
    return false;
  return value.state === undefined || isGameState(value.state);
}

export function isGameState(state: unknown): state is GameState {
  return (
    object(state) &&
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
    view.state !== undefined &&
    now >= view.updated_at &&
    now - view.updated_at < GameLimits.StateFreshSeconds
  );
}
