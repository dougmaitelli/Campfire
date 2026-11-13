import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { Server } from "../../api/servers";
import {
  activityFilterFromQuery,
  activityInProgress,
  activityQuery,
  readActivity,
  readActivityChoices,
  serverActivityURL,
} from "../../api/serverActivity";
import { API_READ_TIMEOUT_MS } from "../../api/client";
import { useCurrentRoute } from "../../routing/useCurrentPage";
import { useCollection } from "../../hooks/useCollection";
import { ActivityFilters } from "../activity/ActivityFilters";
import { ActivityTable } from "../activity/ActivityTable";
import { ActivityDetailsDrawer } from "../activity/ActivityDetailsDrawer";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import "../activity/server-activity.css";

export function ServerActivity({ server }: { server: Server }) {
  const { t } = useTranslation();
  const route = useCurrentRoute();
  const query = useMemo(
    () => new URLSearchParams(route.split("?")[1] ?? ""),
    [route],
  );
  const filter = useMemo(() => activityFilterFromQuery(query), [query]);
  const signature = activityQuery(filter).toString();
  const entry = query.get("entry") ?? "";
  const [poll, setPoll] = useState(10000);
  const load = useCallback(
    async (signal: AbortSignal) => {
      const value = await readActivity(
        server.id,
        activityFilterFromQuery(new URLSearchParams(signature)),
        AbortSignal.any([signal, AbortSignal.timeout(API_READ_TIMEOUT_MS)]),
      );
      if (!signal.aborted)
        setPoll(value.items.some(activityInProgress) ? 2000 : 10000);
      return [{ id: signature, ...value }];
    },
    [server.id, signature],
  );
  const { items, loading, error, connectionError, refresh } = useCollection(
    load,
    filter.before && poll !== 2000 ? undefined : poll,
  );
  const currentRefresh = useRef<typeof refresh | null>(null);
  useEffect(() => {
    currentRefresh.current = refresh;
    return () => {
      currentRefresh.current = null;
    };
  }, [refresh]);
  async function refreshAfterChange() {
    await currentRefresh.current?.();
  }
  const page = items.find((item) => item.id === signature);
  const gameFilters = filter.kind === "game";
  const loadChoices = useCallback(
    async (signal: AbortSignal) =>
      gameFilters
        ? [
            {
              id: server.id,
              ...(await readActivityChoices(
                server.id,
                AbortSignal.any([
                  signal,
                  AbortSignal.timeout(API_READ_TIMEOUT_MS),
                ]),
              )),
            },
          ]
        : [],
    [server.id, gameFilters],
  );
  const choices = useCollection(loadChoices, 30000);
  const open = (key: string) => {
    window.location.hash = serverActivityURL(server.id, filter, key);
  };
  return (
    <section className="server-activity" aria-label={t("serverActivity.title")}>
      <h2>{t("serverActivity.title")}</h2>
      <ActivityFilters
        key={signature}
        filter={filter}
        choices={choices.items.find((item) => item.id === server.id)}
        onApply={(next) => {
          window.location.hash = serverActivityURL(server.id, next);
        }}
      />
      {choices.error && (
        <InlineMessage tone="warning">
          {choices.error}
          <Button onClick={() => void choices.refresh()}>
            {t("common.tryAgain")}
          </Button>
        </InlineMessage>
      )}
      {!!error && !!page?.items.length && (
        <InlineMessage role="alert">
          {error}
          <Button onClick={() => void refresh()}>{t("common.tryAgain")}</Button>
        </InlineMessage>
      )}
      <ActivityTable
        items={page?.items ?? []}
        loading={loading || (!page && !error && !connectionError)}
        error={error}
        onOpen={open}
        onRetry={() => void refresh()}
      />
      <div className="activity-actions">
        {!!filter.before && (
          <Button
            onClick={() => {
              window.location.hash = serverActivityURL(server.id, {
                ...filter,
                before: undefined,
              });
            }}
          >
            {t("serverActivity.newest")}
          </Button>
        )}
        {!!page?.next && (
          <Button
            disabled={loading}
            onClick={() => {
              window.location.hash = serverActivityURL(server.id, {
                ...filter,
                before: page.next,
              });
            }}
          >
            {t("serverActivity.older")}
          </Button>
        )}
      </div>
      {entry && (
        <ActivityDetailsDrawer
          key={entry}
          server={server}
          entryKey={entry}
          onOpen={open}
          onChanged={refreshAfterChange}
          onClose={() => {
            window.location.hash = serverActivityURL(server.id, filter);
          }}
        />
      )}
    </section>
  );
}
