import { RuntimeState } from "../../contracts/domains";
import { useTranslation } from "react-i18next";
import { gameStateFresh, type GameStateView } from "../../api/gameState";
import { gameFieldLabel } from "../../api/gameFields";
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
    usage?.state === RuntimeState.Running &&
    (runtimeState ?? usage.state) === "running" &&
    containerFresh(usage, elapsed) &&
    gameStateFresh(game, now);
  const state = available ? game.state : undefined;
  const updated = t("gameState.lastUpdated", {
    time: new Date(game.updated_at * 1000).toLocaleString(),
  });
  return (
    <div
      className="game-state-readings"
      role="group"
      aria-label={t("gameState.gameData")}
      title={updated}
      aria-description={updated}
      tabIndex={0}
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
              <dt>{gameFieldLabel(state.field_labels, key)}</dt>
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
    </div>
  );
}
