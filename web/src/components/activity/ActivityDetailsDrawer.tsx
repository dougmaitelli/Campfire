import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  activityInProgress,
  readActivityDetails,
  retryActivityNotification,
} from "../../api/serverActivity";
import type { Server } from "../../api/servers";
import { changeJob } from "../../api/jobs";
import { retryExecution } from "../../api/activity";
import { dismissLogAlert } from "../../api/logAlerts";
import { API_READ_TIMEOUT_MS } from "../../api/client";
import { useCollection } from "../../hooks/useCollection";
import { useConfirmation } from "../../hooks/useConfirmation";
import { canPerformServer } from "../../permissions";
import { useResourceLinks } from "../../routing/useResourceLinks";
import { LogAction } from "../../contracts/logAlerts";
import { Drawer } from "../layout/Drawer";
import { DetailsList, DetailItem } from "../layout/DetailsList";
import { ContentState } from "../layout/ContentState";
import { InlineMessage } from "../status/InlineMessage";
import { Button, ButtonLink } from "../forms/Button";
import { Table } from "../table/Table";
import { ResourceLink } from "../navigation/ResourceLink";
import { JobRow } from "../jobs/JobRow";
import { ActivityStatus, activityTitle } from "./ActivityStatus";
import { ActivityNotifications } from "./ActivityNotifications";
import {
  GameEventDetails,
  GameFields,
  GameMediaGallery,
} from "./GameEventDetails";
import "../jobs/job-list.css";

export function ActivityDetailsDrawer({
  server,
  entryKey,
  onClose,
  onOpen,
  onChanged,
}: {
  server: Server;
  entryKey: string;
  onClose: () => void;
  onOpen: (key: string) => void;
  onChanged: () => Promise<void>;
}) {
  const { t, i18n } = useTranslation();
  const links = useResourceLinks();
  const confirmation = useConfirmation();
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState("");
  const pending = useRef(false);
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);
  const [poll, setPoll] = useState(2000);
  const load = useCallback(
    async (signal: AbortSignal) => {
      const value = await readActivityDetails(
        server.id,
        entryKey,
        AbortSignal.any([signal, AbortSignal.timeout(API_READ_TIMEOUT_MS)]),
      );
      if (!signal.aborted) setPoll(activityInProgress(value) ? 2000 : 10000);
      return [{ ...value, id: value.key }];
    },
    [server.id, entryKey],
  );
  const { items, loading, error, refresh } = useCollection(load, poll);
  const details = items[0];
  async function change<T>(
    action: () => Promise<T>,
    onSuccess?: (value: T) => void,
  ) {
    const route = window.location.hash;
    const isCurrent = () => mounted.current && window.location.hash === route;
    if (!isCurrent() || pending.current) return;
    pending.current = true;
    setBusy(true);
    setActionError("");
    try {
      const result = await action();
      await onChanged();
      if (!isCurrent()) return;
      if (onSuccess) onSuccess(result);
      else await refresh();
    } catch (cause) {
      if (!isCurrent()) return;
      setActionError(
        cause instanceof Error
          ? cause.message
          : t("client.unableToCompleteTheRequest"),
      );
    } finally {
      pending.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  async function retryOperation() {
    if (
      pending.current ||
      !details?.operation ||
      !(await confirmation.confirm({
        title: t("confirmDialog.retryOperation"),
        description: t(
          "schedulesPage.retryThisOperationVerifyTheCurrentServerStateFirstAn",
        ),
        confirmLabel: t("common.tryAgain"),
        warning: true,
      }))
    )
      return;
    await change(
      () => retryExecution(details.operation!.id),
      (attempt) => onOpen(`operation:${attempt.id}`),
    );
  }
  const date = (value: number) =>
    new Date(value * 1000).toLocaleString(i18n.language);
  return (
    <>
      <Drawer
        title={details ? activityTitle(details) : t("serverActivity.details")}
        busy={busy}
        onClose={onClose}
        footerLeft={
          <Button onClick={onClose} disabled={busy}>
            {t("common.close")}
          </Button>
        }
        footerRight={null}
      >
        <div className="panel-content activity-details">
          {actionError && (
            <InlineMessage role="alert">{actionError}</InlineMessage>
          )}
          {error && details && (
            <InlineMessage role="alert">
              {error}
              <Button onClick={() => void refresh()}>
                {t("common.tryAgain")}
              </Button>
            </InlineMessage>
          )}
          {!details ? (
            <ContentState
              loading={loading}
              error={error}
              resource={t("serverActivity.details")}
              onRetry={() => void refresh()}
            />
          ) : (
            <>
              {!details.job && (
                <DetailsList>
                  <DetailItem label={t("serverActivity.type")}>
                    {t(`serverActivity.kinds.${details.kind}`)}
                  </DetailItem>
                  <DetailItem label={t("serverActivity.recorded")}>
                    {date(details.created_at)}
                  </DetailItem>
                  {!!details.state && (
                    <DetailItem label={t("common.status")}>
                      <ActivityStatus state={details.state} />
                    </DetailItem>
                  )}
                </DetailsList>
              )}
              {details.source_key &&
                (details.source_available ? (
                  <Button onClick={() => onOpen(details.source_key!)}>
                    {t("serverActivity.viewSource")}
                  </Button>
                ) : (
                  <p className="field-help">
                    {t("serverActivity.sourceUnavailable")}
                  </p>
                ))}
              {details.game_event && (
                <GameEventDetails event={details.game_event} />
              )}
              {details.monitoring_alert && (
                <p>{details.monitoring_alert.body}</p>
              )}
              {details.alert && (
                <>
                  <p>{details.alert.message}</p>
                  {!!details.alert.line && (
                    <pre className="activity-evidence">
                      {details.alert.line}
                    </pre>
                  )}
                  {details.alert.action?.type === LogAction.OpenFile &&
                    canPerformServer(server, "readFiles") && (
                      <ButtonLink
                        href={`#servers/${encodeURIComponent(server.id)}/files/${encodeURIComponent(details.alert.action.path ?? "")}`}
                      >
                        {t("serverActivity.openFile")}
                      </ButtonLink>
                    )}
                  {details.state === "active" &&
                    canPerformServer(server, "operate") && (
                      <Button
                        disabled={busy}
                        onClick={() =>
                          void change(() =>
                            dismissLogAlert(server.id, details.alert!.id),
                          )
                        }
                      >
                        {t("serverActivity.dismiss")}
                      </Button>
                    )}
                </>
              )}
              {details.operation && (
                <>
                  <DetailsList>
                    <DetailItem label={t("serverActivity.initiator")}>
                      <ResourceLink
                        href={links.user(details.operation.user_id)}
                      >
                        {details.operation.username}
                      </ResourceLink>
                    </DetailItem>
                    <DetailItem label={t("serverActivity.trigger")}>
                      {t(
                        details.operation.schedule_id
                          ? "serverActivity.scheduled"
                          : "serverActivity.manual",
                      )}
                    </DetailItem>
                    {!!details.operation.started_at && (
                      <DetailItem label={t("serverActivity.started")}>
                        {date(details.operation.started_at)}
                      </DetailItem>
                    )}
                    {!!details.operation.finished_at && (
                      <DetailItem label={t("serverActivity.finished")}>
                        {date(details.operation.finished_at)}
                      </DetailItem>
                    )}
                  </DetailsList>
                  {details.operation.message && (
                    <p>{details.operation.message}</p>
                  )}
                  {details.can_retry_operation && (
                    <Button
                      disabled={busy}
                      onClick={() => void retryOperation()}
                    >
                      {t("common.tryAgain")}
                    </Button>
                  )}
                </>
              )}
              {details.job && (
                <Table
                  aria-label={t("common.jobs")}
                  className="jobs-table table-stacked"
                >
                  <thead>
                    <tr>
                      <th>{t("serverJobs.jobColumn")}</th>
                      <th>{t("common.status")}</th>
                      <th>{t("serverJobs.created")}</th>
                      <th className="table-actions">
                        <span className="visually-hidden">
                          {t("common.actions")}
                        </span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    <JobRow
                      job={details.job}
                      serverID={server.id}
                      busy={busy}
                      onChange={async (job, action) =>
                        change(() => changeJob(job.id, action))
                      }
                    />
                  </tbody>
                </Table>
              )}
              {details.notification && (
                <>
                  <DetailsList>
                    <DetailItem label={t("serverActivity.provider")}>
                      {details.notification.name}
                    </DetailItem>
                    {!!details.destination && (
                      <DetailItem label={t("serverActivity.destination")}>
                        #{details.destination}
                      </DetailItem>
                    )}
                    <DetailItem label={t("serverActivity.attempts")}>
                      {details.notification.attempts}
                    </DetailItem>
                    {!!details.notification.delivered_at && (
                      <DetailItem label={t("serverActivity.delivered")}>
                        {date(details.notification.delivered_at)}
                      </DetailItem>
                    )}
                    {!!details.notification.next_attempt_at && (
                      <DetailItem label={t("serverActivity.nextAttempt")}>
                        {date(details.notification.next_attempt_at)}
                      </DetailItem>
                    )}
                  </DetailsList>
                  {details.notification.body && (
                    <p>{details.notification.body}</p>
                  )}
                  {details.notification.data && (
                    <GameFields
                      data={details.notification.data}
                      labels={details.notification.field_labels}
                    />
                  )}
                  {details.notification.attachment &&
                    details.source_available && (
                      <GameMediaGallery
                        serverID={server.id}
                        eventID={details.notification.attachment.game_event_id}
                        attachments={[details.notification.attachment.media]}
                      />
                    )}
                  {details.notification.error && (
                    <InlineMessage tone="warning">
                      {details.notification.error}
                    </InlineMessage>
                  )}
                  <div className="activity-actions">
                    {details.notification.can_retry && (
                      <Button
                        disabled={busy}
                        onClick={() =>
                          void change(() =>
                            retryActivityNotification(server.id, entryKey),
                          )
                        }
                      >
                        {t(
                          details.state === "uncertain"
                            ? "serverActivity.checkDelivery"
                            : "common.tryAgain",
                        )}
                      </Button>
                    )}
                    {details.notification.external_url && (
                      <a
                        href={details.notification.external_url}
                        target="_blank"
                        rel="noreferrer"
                      >
                        {t("serverActivity.openMessage")}
                      </a>
                    )}
                  </div>
                </>
              )}
              {details.notification_count > 0 && (
                <ActivityNotifications
                  serverID={server.id}
                  entryKey={entryKey}
                  onOpen={onOpen}
                />
              )}
            </>
          )}
        </div>
      </Drawer>
      {confirmation.dialog}
    </>
  );
}
