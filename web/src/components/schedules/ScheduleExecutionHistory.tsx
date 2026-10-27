import { useCallback, useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  activityCursor,
  listExecutions,
  type Execution,
} from "../../api/activity";
import { useCollection } from "../../hooks/useCollection";
import { useResourceLinks } from "../../routing/useResourceLinks";
import { Button } from "../forms/Button";
import { ResourceLink } from "../navigation/ResourceLink";
import { InlineMessage } from "../status/InlineMessage";
import { Table } from "../table/Table";
import { ExecutionState } from "./ExecutionState";

export function ScheduleExecutionHistory({
  scheduleID,
}: {
  scheduleID: string;
}) {
  const { t } = useTranslation();
  const titleID = useId();
  const links = useResourceLinks();
  const load = useCallback(
    (signal: AbortSignal) => listExecutions(signal, "", "", scheduleID),
    [scheduleID],
  );
  const history = useCollection(load, 10000);
  const [older, setOlder] = useState<Execution[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const active = useRef<AbortController | null>(null);
  useEffect(() => () => active.current?.abort(), []);
  const items = [
    ...new Map(
      [...older, ...history.items].map((run) => [run.id, run]),
    ).values(),
  ].sort((a, b) => b.created_at - a.created_at || b.id.localeCompare(a.id));
  async function more() {
    if (active.current) return;
    const controller = new AbortController();
    active.current = controller;
    setLoading(true);
    setError(null);
    try {
      const next = await listExecutions(
        controller.signal,
        activityCursor(items.at(-1)),
        "",
        scheduleID,
      );
      if (!controller.signal.aborted) {
        setOlder((previous) => [...previous, ...history.items, ...next]);
        setHasMore(next.length === 100);
      }
    } catch (cause) {
      if (!controller.signal.aborted)
        setError(
          cause instanceof Error
            ? cause.message
            : t("schedulesPage.unableToLoadHistory"),
        );
    } finally {
      if (!controller.signal.aborted) setLoading(false);
      if (active.current === controller) active.current = null;
    }
  }
  const when = (value: number) =>
    value ? new Date(value * 1000).toLocaleString() : "—";
  return (
    <section aria-labelledby={titleID}>
      <h3 id={titleID}>{t("schedulesPage.executionHistory")}</h3>
      {items.length > 0 && history.error && (
        <InlineMessage role="alert">{history.error}</InlineMessage>
      )}
      <Table
        empty={items.length === 0}
        loading={history.loading && !items.length}
        error={history.error}
        resource={t("schedulesPage.executions")}
        emptyMessage={t("schedulesPage.noExecutionsRecorded")}
        onRetry={() => void history.refresh()}
      >
        <caption className="visually-hidden">
          {t("schedulesPage.executionHistory")}
        </caption>
        <thead>
          <tr>
            <th>{t("schedulesPage.created")}</th>
            <th>{t("schedulesPage.actor")}</th>
            <th>{t("schedulesPage.state")}</th>
          </tr>
        </thead>
        <tbody>
          {items.map((run) => (
            <tr key={run.id}>
              <td>
                {when(run.created_at)}
                {run.finished_at > 0 && (
                  <span className="row-description">
                    {t("schedulesPage.finished")}: {when(run.finished_at)}
                  </span>
                )}
              </td>
              <td>
                <ResourceLink href={links.user(run.user_id)}>
                  {run.username}
                </ResourceLink>
              </td>
              <td>
                <ExecutionState state={run.state} />
                {run.message && (
                  <span className="row-description">{run.message}</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </Table>
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
      {history.items.length === 100 && hasMore && (
        <Button
          disabled={loading}
          loading={loading}
          onClick={() => void more()}
        >
          {t("schedulesPage.loadOlderExecutions")}
        </Button>
      )}
    </section>
  );
}
