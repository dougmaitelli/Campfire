import { useCallback, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import {
  gameMediaURL,
  readGameEvents,
  type GameEventFilter,
} from "../../api/gameEvents";
import { useCollection } from "../../hooks/useCollection";
import { Button } from "../forms/Button";
import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { InlineMessage } from "../status/InlineMessage";
import { ContentState } from "../layout/ContentState";
import { ListToolbar } from "../layout/ListToolbar";
import "./game-events.css";

export function ServerGameEvents({ serverID }: { serverID: string }) {
  const { t } = useTranslation();
  const [error, setError] = useState<string | null>(null);
  const [type, setType] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [filter, setFilter] = useState<GameEventFilter>({});
  function apply(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const start = from
      ? Math.floor(new Date(from).getTime() / 1000)
      : undefined;
    const end = to ? Math.floor(new Date(to).getTime() / 1000) : undefined;
    if (
      (start !== undefined && !Number.isFinite(start)) ||
      (end !== undefined && !Number.isFinite(end)) ||
      (start !== undefined && end !== undefined && start >= end)
    ) {
      setError(t("gameEvents.invalidRange"));
      return;
    }
    setError(null);
    setFilter({
      ...(type.trim() ? { type: type.trim() } : {}),
      ...(start !== undefined ? { from: start } : {}),
      ...(end !== undefined ? { to: end } : {}),
    });
  }
  return (
    <section className="game-events" aria-label={t("gameEvents.title")}>
      <h2>{t("gameEvents.title")}</h2>
      <form onSubmit={apply}>
        <ListToolbar>
          <FormField label={t("gameEvents.type")}>
            <TextInput
              value={type}
              onChange={(event) => setType(event.target.value)}
              maxLength={64}
              placeholder={t("gameEvents.allTypes")}
            />
          </FormField>
          <FormField label={t("gameEvents.from")}>
            <TextInput
              type="datetime-local"
              value={from}
              onChange={(event) => setFrom(event.target.value)}
            />
          </FormField>
          <FormField label={t("gameEvents.to")}>
            <TextInput
              type="datetime-local"
              value={to}
              onChange={(event) => setTo(event.target.value)}
            />
          </FormField>
          <Button type="submit">{t("gameEvents.apply")}</Button>
          <Button
            type="button"
            onClick={() => {
              setType("");
              setFrom("");
              setTo("");
              setError(null);
              setFilter({});
            }}
          >
            {t("gameEvents.clear")}
          </Button>
        </ListToolbar>
      </form>
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
      <GameEventResults
        key={`${serverID}:${JSON.stringify(filter)}`}
        serverID={serverID}
        filter={filter}
        onPage={(before) => {
          const next = { ...filter };
          if (before === undefined) delete next.before;
          else next.before = before;
          setError(null);
          setFilter(next);
        }}
      />
    </section>
  );
}

function GameEventResults({
  serverID,
  filter,
  onPage,
}: {
  serverID: string;
  filter: GameEventFilter;
  onPage: (before: number | undefined) => void;
}) {
  const { t } = useTranslation();
  const [nextBefore, setNextBefore] = useState<number | undefined>();
  const load = useCallback(
    async (signal: AbortSignal) => {
      const page = await readGameEvents(
        serverID,
        filter,
        AbortSignal.any([signal, AbortSignal.timeout(15000)]),
      );
      if (!signal.aborted) setNextBefore(page.next_before);
      return page.items;
    },
    [serverID, filter],
  );
  const { items, loading, error, refresh } = useCollection(
    load,
    filter.before === undefined ? 10000 : undefined,
  );
  return (
    <>
      {error && (
        <InlineMessage role="alert">
          {error}
          <Button onClick={() => void refresh()}>
            {t("gameEvents.retry")}
          </Button>
        </InlineMessage>
      )}
      {loading && <ContentState loading resource={t("gameEvents.title")} />}
      {!loading && !error && items.length === 0 && (
        <ContentState
          resource={t("gameEvents.title")}
          emptyMessage={t("gameEvents.empty")}
          emptyDescription={t("gameEvents.emptyDescription")}
        />
      )}
      <ol className="game-event-list">
        {!loading &&
          !error &&
          items.map((event) => (
            <li key={event.id}>
              <strong>{event.title}</strong>
              <span className="game-event-type">{event.type}</span>
              {event.attachments && event.attachments.length > 0 && (
                <div className="game-event-media">
                  {event.attachments.map((attachment) => {
                    const url = gameMediaURL(serverID, event.id, attachment.id);
                    return (
                      <figure key={attachment.id}>
                        {attachment.content_type.startsWith("image/") ? (
                          <a href={url} target="_blank" rel="noreferrer">
                            <img
                              src={url}
                              alt={attachment.name}
                              loading="lazy"
                            />
                          </a>
                        ) : (
                          <video
                            controls
                            preload="none"
                            aria-label={attachment.name}
                          >
                            <source src={url} type={attachment.content_type} />
                            <a href={`${url}?download=1`}>{attachment.name}</a>
                          </video>
                        )}
                        <figcaption>
                          <a
                            href={`${url}?download=1`}
                            download={attachment.name}
                          >
                            {t("gameEvents.downloadMedia", {
                              name: attachment.name,
                            })}
                          </a>
                          <span>
                            {t("gameEvents.mediaSize", {
                              size: (attachment.size / 1024 / 1024).toFixed(1),
                            })}
                          </span>
                        </figcaption>
                      </figure>
                    );
                  })}
                </div>
              )}
              <div className="game-event-times">
                <span>
                  {t("gameEvents.occurred")}{" "}
                  <time dateTime={event.occurred_at}>
                    {new Date(event.occurred_at).toLocaleString()}
                  </time>
                </span>
                <span>
                  {t("gameEvents.received")}{" "}
                  <time
                    dateTime={new Date(event.received_at * 1000).toISOString()}
                  >
                    {new Date(event.received_at * 1000).toLocaleString()}
                  </time>
                </span>
              </div>
              {Object.keys(event.data).length > 0 && (
                <dl>
                  {Object.entries(event.data).map(([key, value]) => (
                    <div key={key}>
                      <dt>{key.replace(/[_.-]+/g, " ")}</dt>
                      <dd>
                        {typeof value === "boolean"
                          ? t(value ? "gameState.yes" : "gameState.no")
                          : String(value)}
                      </dd>
                    </div>
                  ))}
                </dl>
              )}
            </li>
          ))}
      </ol>
      <div className="game-event-pagination">
        {filter.before !== undefined && (
          <Button
            onClick={() => {
              onPage(undefined);
            }}
          >
            {t("gameEvents.newest")}
          </Button>
        )}
        {!error && nextBefore !== undefined && (
          <Button
            disabled={loading}
            onClick={() => {
              onPage(nextBefore);
            }}
          >
            {t("gameEvents.older")}
          </Button>
        )}
      </div>
    </>
  );
}
