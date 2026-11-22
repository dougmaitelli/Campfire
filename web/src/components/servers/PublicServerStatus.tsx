import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, Copy, Server } from "lucide-react";
import type { PublicStatusEntry } from "../../api/statusPages";
import { PublicServerState } from "../../contracts/domains";
import { gameStateFresh } from "../../api/gameState";
import { PublicStatusPolicy } from "../../config/policies";
import { presentGameFields } from "../../utils/gameFieldPresentation";
import { GameArt } from "../media/GameArt";
import { StatusPill } from "../status/StatusPill";
import { GameFieldReading } from "./GameField";
import { AvailabilityHistory } from "./AvailabilityHistory";
import "./game-state-readings.css";

// Presentation consumes only the public projection, never private server APIs.
export function PublicServerStatus({
  entry,
  now,
  failed,
}: {
  entry: PublicStatusEntry;
  now: number;
  failed: boolean;
}) {
  const { t, i18n } = useTranslation();
  const [copied, setCopied] = useState<string>();
  const [copyFailed, setCopyFailed] = useState(false);
  const age = now - entry.observed_at;
  const state =
    failed ||
    entry.observed_at <= 0 ||
    age < 0 ||
    age >= PublicStatusPolicy.FreshSeconds
      ? PublicServerState.Unknown
      : entry.state;
  const game =
    state === PublicServerState.Online &&
    entry.game_state &&
    gameStateFresh(entry.game_state, now)
      ? entry.game_state.state
      : undefined;
  const copiedAddress = !!entry.game_address && copied === entry.game_address;
  const copyLabel = t(
    copiedAddress ? "statusPages.addressCopied" : "statusPages.copyAddress",
    { name: entry.name },
  );
  async function copyAddress() {
    if (!entry.game_address) return;
    try {
      await navigator.clipboard.writeText(entry.game_address);
      setCopied(entry.game_address);
      setCopyFailed(false);
    } catch {
      setCopyFailed(true);
    }
  }
  return (
    <li className="public-status-entry">
      {entry.history && <AvailabilityHistory history={entry.history} />}
      <span className="public-status-server-icon" aria-hidden="true">
        <Server size={24} />
        <GameArt src={entry.art_url} />
      </span>
      <div className="public-status-server-details">
        <h3>{entry.name}</h3>
        {entry.game_address && (
          <p className="public-status-address">
            <span>{entry.game_address}</span>
            <button
              type="button"
              className="public-status-copy"
              aria-label={copyLabel}
              title={copyLabel}
              onClick={() => void copyAddress()}
            >
              {copiedAddress ? (
                <Check size="1em" aria-hidden="true" />
              ) : (
                <Copy size="1em" aria-hidden="true" />
              )}
            </button>
          </p>
        )}
        <span className="visually-hidden" role="status">
          {copiedAddress ? copyLabel : ""}
        </span>
        {copyFailed && (
          <p role="status">{t("statusPages.addressCopyFailed")}</p>
        )}
      </div>
      <StatusPill
        tone={
          state === PublicServerState.Online
            ? "positive"
            : state === PublicServerState.Offline
              ? "negative"
              : "neutral"
        }
      >
        {t(`statusPages.states.${state}`)}
      </StatusPill>
      <div className="public-status-footer">
        {entry.game_state && (
          <div
            className="game-state-readings public-status-game-state"
            role="group"
            aria-label={t("gameState.gameData")}
          >
            {game ? (
              <dl>
                {presentGameFields(
                  game,
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
        )}
        <p className="public-status-checked">
          {entry.observed_at > 0
            ? t("statusPages.observed", {
                time: new Date(entry.observed_at * 1000).toLocaleString(),
              })
            : t("statusPages.noObservation")}
        </p>
      </div>
    </li>
  );
}
