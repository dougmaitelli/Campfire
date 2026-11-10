import { useTranslation } from "react-i18next";
import {
  gameMediaURL,
  type GameEvent,
  type GameAttachment,
} from "../../api/gameEvents";
import { gameFieldLabel, type GameFieldLabels } from "../../api/gameFields";
import { formatBytes } from "../../utils/formatBytes";
import { DetailsList, DetailItem } from "../layout/DetailsList";

export function GameFields({
  data,
  labels,
}: {
  data: Record<string, string | number | boolean>;
  labels?: GameFieldLabels | undefined;
}) {
  const { t } = useTranslation();
  return (
    <DetailsList>
      {Object.entries(data).map(([key, value]) => (
        <DetailItem key={key} label={gameFieldLabel(labels, key)}>
          {typeof value === "boolean"
            ? t(value ? "gameState.yes" : "gameState.no")
            : String(value)}
        </DetailItem>
      ))}
    </DetailsList>
  );
}

export function GameMediaGallery({
  serverID,
  eventID,
  attachments,
}: {
  serverID: string;
  eventID: number;
  attachments: GameAttachment[];
}) {
  const { t } = useTranslation();
  return (
    <div className="activity-media">
      {attachments.map((file) => {
        const url = gameMediaURL(serverID, eventID, file.id);
        return (
          <figure key={file.id}>
            {file.content_type.startsWith("image/") ? (
              <a href={url} target="_blank" rel="noreferrer">
                <img src={url} alt={file.name} loading="lazy" />
              </a>
            ) : (
              <video controls preload="metadata" aria-label={file.name}>
                <source src={url} type={file.content_type} />
                <a href={`${url}?download=1`}>{file.name}</a>
              </video>
            )}
            <figcaption>
              <a href={`${url}?download=1`} download={file.name}>
                {t("gameEvents.downloadMedia", { name: file.name })}
              </a>
              <span>{formatBytes(file.size)}</span>
            </figcaption>
          </figure>
        );
      })}
    </div>
  );
}

export function GameEventDetails({ event }: { event: GameEvent }) {
  const { t, i18n } = useTranslation();
  return (
    <>
      <DetailsList>
        <DetailItem label={t("gameEvents.type")}>{event.type}</DetailItem>
        <DetailItem label={t("gameEvents.occurred")}>
          {new Date(event.occurred_at).toLocaleString(i18n.language)}
        </DetailItem>
      </DetailsList>
      <GameFields data={event.data} labels={event.field_labels} />
      {!!event.attachments?.length && (
        <GameMediaGallery
          serverID={event.server_id}
          eventID={event.id}
          attachments={event.attachments}
        />
      )}
    </>
  );
}
