import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { JobStatus } from "../../contracts/domains";
import { API_READ_TIMEOUT_MS } from "../../api/client";
import {
  listJobs,
  changeJob,
  type ServerJob,
  type JobFilter,
} from "../../api/jobs";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import { Table } from "../table/Table";
import { JobRow } from "./JobRow";
import i18n from "../../i18n";
import "./job-list.css";

async function unchanged() {}
function isFinished(job: ServerJob) {
  return (
    job.status === JobStatus.Completed || job.status === JobStatus.Cancelled
  );
}

export function JobList({
  serverID,
  filter,
  onChanged = unchanged,
}: {
  serverID?: string | undefined;
  filter?: JobFilter | undefined;
  onChanged?: (() => Promise<void>) | undefined;
}) {
  const { t } = useTranslation();
  const type = filter?.type;
  const status = filter?.status;
  const [jobs, setJobs] = useState<ServerJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [pages, setPages] = useState(1);
  const [next, setNext] = useState("");
  const [revision, setRevision] = useState(0);
  const pending = useRef(false);
  const active = useRef(new Set<string>());
  useEffect(() => {
    const controller = new AbortController();
    const history = new Map<string, Awaited<ReturnType<typeof listJobs>>>();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function poll() {
      let delay = 30_000;
      try {
        const signal = AbortSignal.any([
          controller.signal,
          AbortSignal.timeout(API_READ_TIMEOUT_MS),
        ]);
        const items: ServerJob[] = [];
        let cursor = "";
        for (let page = 0; page < pages; page++) {
          const cached = history.get(cursor);
          // Status filters can gain older jobs as they finish, even when all
          // previously cached rows are terminal.
          const result =
            page > 0 && !status && cached && cached.items.every(isFinished)
              ? cached
              : await listJobs(serverID, signal, cursor || undefined, {
                  type,
                  status,
                });
          history.set(cursor, result);
          items.push(...result.items);
          cursor = result.next;
          if (!cursor) break;
        }
        if (!controller.signal.aborted && !pending.current) {
          const finished = items.some(
            (job) => active.current.has(job.id) && isFinished(job),
          );
          active.current = new Set(
            items.filter((job) => !isFinished(job)).map((job) => job.id),
          );
          setJobs(items);
          setNext(cursor);
          setLoading(false);
          setError(null);
          if (finished) await onChanged();
        }
        if (
          items.some(
            (job) => !isFinished(job) && job.status !== JobStatus.Failed,
          )
        )
          delay = 2000;
      } catch (cause) {
        if (!controller.signal.aborted) {
          delay = 2000;
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverJobs.unableToLoad"),
          );
          setLoading(false);
        }
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => void poll(), delay);
    }
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [serverID, type, status, pages, revision, onChanged]);
  async function change(job: ServerJob, action: "retry" | "cancel") {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      const updated = await changeJob(job.id, action);
      setJobs((current) =>
        current.map((item) => (item.id === updated.id ? updated : item)),
      );
      setRevision((value) => value + 1);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("serverJobs.unableToChange"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <section
      className={`server-jobs${serverID ? "" : " server-jobs-global"}`}
      aria-label={t("serverDetailsDrawer.jobs")}
      aria-busy={loading}
    >
      {error && jobs.length > 0 && (
        <InlineMessage role="alert">
          {error}
          <Button
            disabled={busy}
            onClick={() => setRevision((value) => value + 1)}
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
        onRetry={() => setRevision((value) => value + 1)}
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
              serverID={serverID}
              busy={busy}
              onChange={change}
            />
          ))}
        </tbody>
      </Table>
      {next && (
        <Button disabled={busy} onClick={() => setPages((value) => value + 1)}>
          {t("serverJobs.loadOlder")}
        </Button>
      )}
    </section>
  );
}
