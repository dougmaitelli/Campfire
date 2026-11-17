import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { JobStatus } from "../../contracts/domains";
import {
  listJobs,
  changeJob,
  type ServerJob,
  type JobFilter,
} from "../../api/jobs";
import { usePagination } from "../../hooks/usePagination";
import { usePagedCollection } from "../../hooks/usePagedCollection";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import { Table } from "../table/Table";
import { Pagination } from "../table/Pagination";
import { JobRow } from "./JobRow";
import "./job-list.css";

async function unchanged() {}
function isFinished(job: ServerJob) {
  return (
    job.status === JobStatus.Completed || job.status === JobStatus.Cancelled
  );
}

export function JobList({
  filter,
  onChanged = unchanged,
}: {
  filter?: JobFilter | undefined;
  onChanged?: (() => Promise<void>) | undefined;
}) {
  const { t } = useTranslation();
  const type = filter?.type;
  const status = filter?.status;
  const navigation = usePagination();
  const active = useRef(new Set<string>());
  const [poll, setPoll] = useState(30000);
  const load = useCallback(
    async (signal: AbortSignal, page: number) => {
      const result = await listJobs(
        undefined,
        signal,
        page,
        { type, status },
        navigation.pageSize,
      );
      if (!signal.aborted) {
        const finished = result.items.some(
          (job) => active.current.has(job.id) && isFinished(job),
        );
        active.current = new Set(
          result.items.filter((job) => !isFinished(job)).map((job) => job.id),
        );
        setPoll(
          result.items.some(
            (job) => !isFinished(job) && job.status !== JobStatus.Failed,
          )
            ? 2000
            : 30000,
        );
        if (finished) await onChanged();
      }
      return result;
    },
    [type, status, onChanged, navigation.pageSize],
  );
  const history = usePagedCollection(load, navigation.page, poll);
  const { items: jobs, loading } = history;
  const [actionError, setError] = useState<string | null>(null);
  const error = actionError ?? history.error;
  const [busy, setBusy] = useState(false);
  const pending = useRef(false);
  async function change(job: ServerJob, action: "retry" | "cancel") {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await changeJob(job.id, action);
      await history.refresh();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("serverJobs.unableToChange"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <section
      className="server-jobs server-jobs-global"
      aria-label={t("common.jobs")}
      aria-busy={loading}
    >
      {error && jobs.length > 0 && (
        <InlineMessage role="alert">
          {error}
          <Button
            disabled={busy}
            onClick={() => {
              setError(null);
              void history.refresh();
            }}
          >
            {t("common.tryAgain")}
          </Button>
        </InlineMessage>
      )}
      <Table
        className="jobs-table table-stacked"
        aria-label={t("common.jobs")}
        empty={jobs.length === 0}
        loading={loading}
        error={error}
        resource={t("common.jobs")}
        emptyMessage={t("serverJobs.empty")}
        emptyIcon="jobs"
        onRetry={() => void history.refresh()}
      >
        <thead>
          <tr>
            <th scope="col">{t("serverJobs.jobColumn")}</th>
            <th scope="col">{t("common.status")}</th>
            <th scope="col">{t("serverJobs.created")}</th>
            <th scope="col" className="table-actions">
              <span className="visually-hidden">{t("common.actions")}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {jobs.map((job) => (
            <JobRow
              key={job.id}
              job={job}
              serverID={undefined}
              busy={busy}
              onChange={change}
            />
          ))}
        </tbody>
      </Table>
      <Pagination
        pagination={history.pagination}
        onPageChange={navigation.setPage}
        onPageSizeChange={navigation.setPageSize}
        loading={loading || busy}
      />
    </section>
  );
}
