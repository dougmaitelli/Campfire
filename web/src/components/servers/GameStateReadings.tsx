import { useTranslation } from "react-i18next";
import { gameStateFresh, type GameStateView } from "../../api/gameState";
import { containerFresh, type ResourceUsage } from "../../api/monitoring";
import "./game-state-readings.css";

export function GameStateReadings({
  game,
  now,
  usage,
  elapsed,
  runtimeState,
  busy = false,
}: {
  game: GameStateView | undefined;
  now: number;
  usage: ResourceUsage | undefined;
  elapsed: number;
  runtimeState?: string | undefined;
  busy?: boolean | undefined;
}) {
  const { t } = useTranslation();
  if (!game) return null;
  const available =
    !busy &&
    usage?.state === "running" &&
    (runtimeState ?? usage.state) === "running" &&
    containerFresh(usage, elapsed) &&
    gameStateFresh(game, now);
  const state = available ? game.state : undefined;
  const updated = new Date(game.updated_at * 1000).toLocaleString();
  const labels = new Map([
    ["map", t("gameState.map")],
    ["game_mode", t("gameState.gameMode")],
  ]);
  return (
    <div
      className="game-state-readings"
      role="group"
      aria-label={t("gameState.gameData")}
    >
      {state ? (
        <dl>
          {state.players !== undefined && (
            <div>
              <dt>{t("gameState.players")}</dt>
              <dd>
                {state.capacity !== undefined
                  ? t("gameState.playersWithCapacity", {
                      players: state.players,
                      capacity: state.capacity,
                    })
                  : state.players.toLocaleString()}
              </dd>
            </div>
          )}
          {state.players === undefined && state.capacity !== undefined && (
            <div>
              <dt>{t("gameState.capacity")}</dt>
              <dd>{state.capacity.toLocaleString()}</dd>
            </div>
          )}
          {Object.entries(state.fields ?? {}).map(([key, value]) => (
            <div key={key}>
              <dt>{labels.get(key) ?? key.replace(/[_.-]+/g, " ")}</dt>
              <dd>
                {typeof value === "boolean"
                  ? t(value ? "gameState.yes" : "gameState.no")
                  : typeof value === "number"
                    ? value.toLocaleString()
                    : value}
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        <span className="game-state-unavailable">
          {t("gameState.unavailable")}
        </span>
      )}
      <span className="game-state-updated">
        {t("gameState.lastUpdated", { time: updated })}
      </span>
    </div>
  );
}
