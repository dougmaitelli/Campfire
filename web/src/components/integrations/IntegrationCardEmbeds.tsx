import type { RenderedIntegrationCard } from "../../api/integrations";
import type { RuntimeState } from "../../contracts/domains";
import { GameFieldReading } from "../servers/GameField";
import "../servers/integration-card-preview.css";

export function IntegrationCardEmbeds({
  preview,
  state,
}: {
  preview: RenderedIntegrationCard;
  state?: RuntimeState;
}) {
  return (
    <>
      {preview.embeds.map((embed, index) => (
        <div className="integration-server-card" data-state={state} key={index}>
          <strong>{embed.title}</strong>
          <dl>
            {embed.fields.map((field, fieldIndex) => (
              <GameFieldReading key={fieldIndex} field={field} unavailable="" />
            ))}
          </dl>
          <span className="field-help">{embed.footer.text}</span>
        </div>
      ))}
    </>
  );
}
