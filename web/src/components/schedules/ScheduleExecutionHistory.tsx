import { useCallback, useId } from "react";
import { useTranslation } from "react-i18next";
import { listExecutions } from "../../api/activity";
import { usePagination } from "../../hooks/usePagination";
import { usePagedCollection } from "../../hooks/usePagedCollection";
import { useResourceLinks } from "../../routing/useResourceLinks";
import { ResourceLink } from "../navigation/ResourceLink";
import { InlineMessage } from "../status/InlineMessage";
import { Table } from "../table/Table";
import { Pagination } from "../table/Pagination";
import { ExecutionState } from "./ExecutionState";

export function ScheduleExecutionHistory({
  scheduleID,
}: {
  scheduleID: string;
}) {
  const { t } = useTranslation();
  const titleID = useId();
  const links = useResourceLinks();
  const navigation = usePagination("execution_page");
  const load = useCallback(
    (signal: AbortSignal, page: number) =>
      listExecutions(signal, page, "", scheduleID),
    [scheduleID],
  );
  const history = usePagedCollection(load, navigation.page);
  const { items } = history;
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
        loading={history.loading}
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
      <Pagination
        pagination={history.pagination}
        onPageChange={navigation.setPage}
        loading={history.loading}
      />
    </section>
  );
}
