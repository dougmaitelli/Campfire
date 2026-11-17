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
import { usePagination } from "../../hooks/usePagination";
import { usePagedCollection } from "../../hooks/usePagedCollection";
import { Pagination } from "../table/Pagination";
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
  const navigation = usePagination();
  const load = useCallback(
    async (signal: AbortSignal, page: number) => {
      const value = await readActivity(
        server.id,
        {
          ...activityFilterFromQuery(new URLSearchParams(signature)),
          page,
          page_size: navigation.pageSize,
        },
        AbortSignal.any([signal, AbortSignal.timeout(API_READ_TIMEOUT_MS)]),
      );
      if (!signal.aborted)
        setPoll(value.items.some(activityInProgress) ? 2000 : 10000);
      return value;
    },
    [server.id, signature, navigation.pageSize],
  );
  const history = usePagedCollection(load, navigation.page, poll);
  const { items, loading, error, refresh } = history;
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
          window.location.hash = serverActivityURL(server.id, {
            ...next,
            page_size: filter.page_size,
          });
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
      {!!error && !!items.length && (
        <InlineMessage role="alert">
          {error}
          <Button onClick={() => void refresh()}>{t("common.tryAgain")}</Button>
        </InlineMessage>
      )}
      <ActivityTable
        items={items}
        loading={loading}
        error={error}
        onOpen={open}
        onRetry={() => void refresh()}
      />
      <Pagination
        pagination={history.pagination}
        onPageChange={navigation.setPage}
        onPageSizeChange={navigation.setPageSize}
        loading={loading}
      />
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
