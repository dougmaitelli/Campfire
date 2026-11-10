import { API_READ_TIMEOUT_MS } from "../../api/client";
import { useCallback, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import {
  gameMediaURL,
  gameEventFieldLabel,
  readGameEvents,
  readGameEventFilterOptions,
  type GameEventFilter,
} from "../../api/gameEvents";
import { gameFieldLabel } from "../../api/gameFields";
import { useCollection } from "../../hooks/useCollection";
import { formatBytes } from "../../utils/formatBytes";
import { Button } from "../forms/Button";
import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { Select } from "../forms/Select";
import { DateTimeInput } from "../forms/DateTimeInput";
import { InlineMessage } from "../status/InlineMessage";
import { ContentState } from "../layout/ContentState";
import { ListToolbar } from "../layout/ListToolbar";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { GameLimits } from "../../config/policies";
import "./game-events.css";

export function ServerGameEvents({ serverID }: { serverID: string }) {
  const { t } = useTranslation();
  const [error, setError] = useState<string | null>(null);
  const [type, setType] = useState("");
  const [search, setSearch] = useState("");
  const [fields, setFields] = useState<
    { id: number; field: string; value: string }[]
  >([]);
  const nextFieldID = useRef(1);
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [filter, setFilter] = useState<GameEventFilter>({});
  const loadOptions = useCallback(
    async (signal: AbortSignal) => [
      {
        id: serverID,
        ...(await readGameEventFilterOptions(
          serverID,
          AbortSignal.any([signal, AbortSignal.timeout(API_READ_TIMEOUT_MS)]),
        )),
      },
    ],
    [serverID],
  );
  const {
    items: optionResults,
    error: optionsError,
    refresh: refreshOptions,
  } = useCollection(loadOptions, 30000);
  const options = optionResults.find((item) => item.id === serverID);
  const fieldNames = Object.keys(options?.fields ?? {}).sort();
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
    const dataFields: Record<string, string> = {};
    for (const entry of fields) {
      const field = entry.field.trim();
      const value = entry.value;
      if (!field && !value) continue;
      if (
        !/^[a-z][a-z0-9._-]{0,63}$/.test(field) ||
        Object.hasOwn(dataFields, field)
      ) {
        setError(t("gameEvents.invalidFields"));
        return;
      }
      dataFields[field] = value;
    }
    setError(null);
    setFilter({
      ...(search.trim() ? { search: search.trim() } : {}),
      ...(Object.keys(dataFields).length ? { fields: dataFields } : {}),
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
          <FormField label={t("gameEvents.search")}>
            <TextInput
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              maxLength={GameLimits.FieldStringLength}
              placeholder={t("gameEvents.searchPlaceholder")}
            />
          </FormField>
          <FormField label={t("gameEvents.type")}>
            <Select value={type} onValueChange={setType} disabled={!options}>
              <option value="">{t("gameEvents.allTypes")}</option>
              {[
                ...new Set([
                  ...(options?.types ?? []),
                  ...(type ? [type] : []),
                ]),
              ].map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </Select>
          </FormField>
          <FormField label={t("gameEvents.from")}>
            <DateTimeInput
              value={from}
              onChange={(event) => setFrom(event.target.value)}
            />
          </FormField>
          <FormField label={t("gameEvents.to")}>
            <DateTimeInput
              value={to}
              onChange={(event) => setTo(event.target.value)}
            />
          </FormField>
          {fields.map((entry, index) => (
            <div className="game-event-field-filter" key={entry.id}>
              <FormField label={t("gameEvents.field")}>
                <Select
                  value={entry.field}
                  disabled={!options}
                  onValueChange={(field) =>
                    setFields((current) =>
                      current.map((item) =>
                        item.id === entry.id
                          ? { ...item, field, value: "" }
                          : item,
                      ),
                    )
                  }
                >
                  <option value="">{t("gameEvents.selectField")}</option>
                  {[
                    ...new Set([
                      ...fieldNames,
                      ...(entry.field ? [entry.field] : []),
                    ]),
                  ].map((field) => (
                    <option
                      key={field}
                      value={field}
                      disabled={fields.some(
                        (item) => item.id !== entry.id && item.field === field,
                      )}
                    >
                      {gameEventFieldLabel(options, type, field)}
                    </option>
                  ))}
                </Select>
              </FormField>
              <FormField label={t("gameEvents.value")}>
                <Select
                  value={entry.value}
                  disabled={!entry.field || !options}
                  onValueChange={(value) =>
                    setFields((current) =>
                      current.map((item) =>
                        item.id === entry.id ? { ...item, value } : item,
                      ),
                    )
                  }
                >
                  <option value="">{t("gameEvents.allValues")}</option>
                  {[
                    ...new Set([
                      ...(options?.fields[entry.field] ?? []),
                      ...(entry.value ? [entry.value] : []),
                    ]),
                  ].map((value) => (
                    <option key={value} value={value}>
                      {value}
                    </option>
                  ))}
                </Select>
              </FormField>
              <Button
                type="button"
                iconOnly
                aria-label={t("gameEvents.removeFieldFilter", {
                  index: index + 1,
                })}
                onClick={() => {
                  setFields((current) =>
                    current.filter((item) => item.id !== entry.id),
                  );
                }}
              >
                <NavigationIcon name="close" />
              </Button>
            </div>
          ))}
          <Button
            type="button"
            disabled={
              fields.length >= GameLimits.Fields ||
              !fieldNames.some(
                (field) => !fields.some((entry) => entry.field === field),
              )
            }
            onClick={() => {
              const entry = { id: nextFieldID.current++, field: "", value: "" };
              setFields((current) =>
                current.length >= GameLimits.Fields
                  ? current
                  : [...current, entry],
              );
            }}
          >
            {t("gameEvents.addFieldFilter")}
          </Button>
          <Button type="submit">{t("gameEvents.apply")}</Button>
          <Button
            type="button"
            onClick={() => {
              setType("");
              setSearch("");
              setFields([]);
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
      {optionsError && (
        <InlineMessage role="alert">
          {optionsError}
          <Button onClick={() => void refreshOptions()}>
            {t("gameEvents.retryOptions")}
          </Button>
        </InlineMessage>
      )}
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
        AbortSignal.any([signal, AbortSignal.timeout(API_READ_TIMEOUT_MS)]),
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
                          <span>{formatBytes(attachment.size)}</span>
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
                      <dt>{gameFieldLabel(event.field_labels, key)}</dt>
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
