import { RuntimeState } from "../../contracts/domains";
import { useTranslation } from "react-i18next";
import { gameStateFresh, type GameStateView } from "../../api/gameState";
import { presentGameFields } from "../../utils/gameFieldPresentation";
import { GameFieldReading } from "./GameField";
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
  const { t, i18n } = useTranslation();
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
          {presentGameFields(
            state,
            undefined,
            t,
            i18n.resolvedLanguage ?? "en",
          ).map((field) => (
            <GameFieldReading
              key={field.id}
              field={field}
              unavailable={t("serverIntegrations.valueUnavailable")}
            />
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
