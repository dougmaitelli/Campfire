import { GameLimits } from "../config/policies";
import { GameStateStatus } from "../contracts/domains";
import { isRecord as object, isEnumValue } from "../utils/validation";
export interface GameState {
  players?: number;
  capacity?: number;
  fields?: Record<string, string | number | boolean>;
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
    (state.fields === undefined ||
      (object(state.fields) &&
        Object.entries(state.fields).length <= GameLimits.Fields &&
        Object.entries(state.fields).every(
          ([key, field]) =>
            /^[a-z][a-z0-9_.-]{0,63}$/.test(key) &&
            (typeof field === "boolean" ||
              (typeof field === "string" &&
                field.length <= GameLimits.FieldStringLength) ||
              (typeof field === "number" && Number.isFinite(field))),
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
