import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  activityInProgress,
  readRelatedNotifications,
} from "../../api/serverActivity";
import { API_READ_TIMEOUT_MS } from "../../api/client";
import { useCollection } from "../../hooks/useCollection";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import { ActivityTable } from "./ActivityTable";

export function ActivityNotifications({
  serverID,
  entryKey,
  onOpen,
}: {
  serverID: string;
  entryKey: string;
  onOpen: (key: string) => void;
}) {
  const { t } = useTranslation();
  const [pages, setPages] = useState(1);
  const [poll, setPoll] = useState(2000);
  const load = useCallback(
    async (signal: AbortSignal) => {
      const requestSignal = AbortSignal.any([
        signal,
        AbortSignal.timeout(API_READ_TIMEOUT_MS),
      ]);
      const value = await readRelatedNotifications(
        serverID,
        entryKey,
        "",
        requestSignal,
      );
      for (let page = 1; page < pages && value.next; page++) {
        const older = await readRelatedNotifications(
          serverID,
          entryKey,
          value.next,
          requestSignal,
        );
        value.items.push(...older.items);
        value.next = older.next;
      }
      if (!signal.aborted)
        setPoll(value.items.some(activityInProgress) ? 2000 : 10000);
      return [{ ...value, id: entryKey, pages }];
    },
    [serverID, entryKey, pages],
  );
  const { items, loading, error, refresh } = useCollection(load, poll);
  const page = items[0];
  return (
    <section aria-label={t("serverActivity.notifications")}>
      <h3>{t("serverActivity.notifications")}</h3>
      {error && !!page?.items.length && (
        <InlineMessage role="alert">
          {error}
          <Button onClick={() => void refresh()}>{t("common.tryAgain")}</Button>
        </InlineMessage>
      )}
      <ActivityTable
        items={page?.items ?? []}
        loading={loading}
        error={error}
        notifications
        onOpen={onOpen}
        onRetry={() => void refresh()}
      />
      {!!page?.next && (
        <Button
          disabled={!error && page.pages < pages}
          loading={!error && page.pages < pages}
          onClick={() => setPages((value) => value + 1)}
        >
          {t("serverActivity.olderNotifications")}
        </Button>
      )}
    </section>
  );
}
