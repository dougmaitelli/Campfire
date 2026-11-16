import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  activityInProgress,
  readRelatedNotifications,
} from "../../api/serverActivity";
import { usePagination } from "../../hooks/usePagination";
import { usePagedCollection } from "../../hooks/usePagedCollection";
import { Pagination } from "../table/Pagination";
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
  const navigation = usePagination("notification_page");
  const [poll, setPoll] = useState(2000);
  const load = useCallback(
    async (signal: AbortSignal, page: number) => {
      const value = await readRelatedNotifications(
        serverID,
        entryKey,
        page,
        signal,
      );
      if (!signal.aborted)
        setPoll(value.items.some(activityInProgress) ? 2000 : 10000);
      return value;
    },
    [serverID, entryKey],
  );
  const history = usePagedCollection(load, navigation.page, poll);
  return (
    <section aria-label={t("serverActivity.notifications")}>
      <h3>{t("serverActivity.notifications")}</h3>
      {history.error && !!history.items.length && (
        <InlineMessage role="alert">
          {history.error}
          <Button onClick={() => void history.refresh()}>
            {t("common.tryAgain")}
          </Button>
        </InlineMessage>
      )}
      <ActivityTable
        items={history.items}
        loading={history.loading}
        error={history.error}
        notifications
        onOpen={onOpen}
        onRetry={() => void history.refresh()}
      />
      <Pagination
        pagination={history.pagination}
        onPageChange={navigation.setPage}
        loading={history.loading}
        label={t("serverActivity.notifications")}
      />
    </section>
  );
}
