import { useTranslation } from "react-i18next";
import { JobStatus, JobType } from "../../contracts/domains";
import type { ServerJob } from "../../api/jobs";
import { useResourceLinks } from "../../routing/useResourceLinks";
import { ResourceLink } from "../navigation/ResourceLink";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { Button } from "../forms/Button";
import { ActionGroup } from "../layout/ActionGroup";
import { ProgressBar } from "../status/ProgressBar";
import { StatusPill } from "../status/StatusPill";
import { formatBytes } from "../../utils/formatBytes";
import { jobStatusLabels, jobStepLabels } from "./jobLabels";

export function JobRow({
  job,
  serverID,
  busy,
  onChange,
}: {
  job: ServerJob;
  serverID: string | undefined;
  busy: boolean;
  onChange: (job: ServerJob, action: "retry" | "cancel") => Promise<void>;
}) {
  const { t, i18n } = useTranslation();
  const links = useResourceLinks();
  const incoming = serverID === job.target_server_id;
  const waiting = job.waiting_for_sparks.length > 0;
  const running =
    !waiting &&
    (job.status === JobStatus.Running || job.status === JobStatus.Pending);
  const type = t(
    job.type === JobType.Copy ? "serverCopy.copy" : "serversPage.transfer",
  );
  return (
    <tr
      role="row"
      className="server-job-history-item"
      aria-label={t("serverJobs.job", { type, id: job.id })}
    >
      <td className="server-job-info">
        <div className="server-job-heading">
          {type}
          {!serverID && (
            <>
              {" "}
              ·{" "}
              <ResourceLink href={links.server(job.source_server_id)}>
                {job.source_name}
              </ResourceLink>
            </>
          )}
          {job.type === JobType.Copy && (
            <>
              {" "}
              {incoming ? "←" : "→"}{" "}
              <ResourceLink
                href={
                  incoming
                    ? links.server(job.source_server_id)
                    : job.status === JobStatus.Cancelled
                      ? undefined
                      : links.server(job.target_server_id)
                }
              >
                {incoming ? job.source_name : job.target_name}
              </ResourceLink>
            </>
          )}
        </div>
        <div className="server-job-route">
          <ResourceLink href={links.spark(job.source_spark_id)}>
            {job.source_spark_name}
          </ResourceLink>
          {" → "}
          <ResourceLink href={links.spark(job.target_spark_id)}>
            {job.target_spark_name}
          </ResourceLink>
        </div>
      </td>
      <td className="server-job-state">
        <div className="server-job-state-content">
          <StatusPill
            tone={
              job.status === JobStatus.Completed
                ? "positive"
                : job.status === JobStatus.Failed
                  ? "negative"
                  : job.status === JobStatus.Cancelled
                    ? "neutral"
                    : "pending"
            }
          >
            {t(
              waiting && job.status !== JobStatus.Cancelling
                ? "serverJobs.waiting"
                : jobStatusLabels[job.status],
            )}
          </StatusPill>
          {waiting && (
            <span className="server-job-phase">
              {t("serverJobs.waitingForSparks", {
                sparks: job.waiting_for_sparks
                  .map((id) =>
                    id === job.source_spark_id
                      ? job.source_spark_name
                      : job.target_spark_name,
                  )
                  .join(", "),
              })}
            </span>
          )}
          {running && (
            <span className="server-job-phase">
              {t(jobStepLabels[job.step])}
            </span>
          )}
          {job.total_bytes > 0 && (
            <div className="server-job-progress">
              {running && (
                <ProgressBar
                  aria-label={t("serverJobs.progress")}
                  aria-valuetext={`${formatBytes(job.transferred_bytes)} / ${formatBytes(job.total_bytes)}`}
                  value={job.transferred_bytes}
                  max={job.total_bytes}
                />
              )}
              <span className="server-job-size">
                {formatBytes(job.transferred_bytes)} /{" "}
                {formatBytes(job.total_bytes)}
              </span>
            </div>
          )}
          {job.error && (
            <span role="alert" className="server-job-error">
              {job.error}
            </span>
          )}
        </div>
      </td>
      <td className="server-job-created">
        <time dateTime={new Date(job.created_at * 1000).toISOString()}>
          {new Date(job.created_at * 1000).toLocaleString(i18n.language)}
        </time>
      </td>
      <td className="table-actions server-job-actions">
        {(job.can_retry || job.can_cancel) && (
          <ActionGroup>
            {job.can_retry && (
              <Button
                iconOnly
                aria-label={t("common.tryAgain")}
                title={t("common.tryAgain")}
                disabled={busy}
                onClick={() => void onChange(job, "retry")}
              >
                <NavigationIcon name="restart" />
              </Button>
            )}
            {job.can_cancel && (
              <Button
                variant="danger"
                iconOnly
                aria-label={t("common.cancel")}
                title={t("common.cancel")}
                disabled={busy}
                onClick={() => void onChange(job, "cancel")}
              >
                <NavigationIcon name="x-circle" />
              </Button>
            )}
          </ActionGroup>
        )}
      </td>
    </tr>
  );
}
