import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { ContentPanel } from "../components/layout/ContentPanel";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { PageHeader } from "../components/layout/PageHeader";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { useEffect, useState } from "react";
import { SparksTable } from "../components/sparks/SparksTable";
import { SparkConnectionState } from "../components/sparks/SparkConnectionState";
import { SparkDialog } from "../dialogs/SparkDialog";
import { EnrollSparkDialog } from "../dialogs/EnrollSparkDialog";
import type { Spark } from "../api/sparks";
import {
  deleteSpark,
  listSparks,
  updateSparkClient,
  pruneSpark,
} from "../api/sparks";
import { useSparks } from "../hooks/useSparks";
import { useMonitoring } from "../hooks/useMonitoring";
import "./sparks.css";

import { SparkDetailsDialog } from "../dialogs/SparkDetailsDialog";
import { useResourceSelection } from "../routing/useResourceSelection";

const UPDATE_RECONNECT_TIMEOUT_MS = 120000;
const UPDATE_STATUS_POLL_INTERVAL_MS = 2000;
const UPDATE_STATUS_REQUEST_TIMEOUT_MS = 10000;

export function SparksPage() {
  const { t } = useTranslation();
  const { sparks, loading, error, refresh, addSpark } = useSparks();
  const { selected: selectedDetails, select: setDetails } =
    useResourceSelection(sparks, !loading && !error);
  const monitoring = useMonitoring();
  const details = selectedDetails
    ? (sparks.find((spark) => spark.id === selectedDetails.id) ??
      selectedDetails)
    : null;
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<Spark | null>(null);
  const [enrolling, setEnrolling] = useState<{
    spark: Spark;
    reEnrollment: boolean;
  } | null>(null);
  const [confirmEnrollment, setConfirmEnrollment] = useState<Spark | null>(
    null,
  );
  const [deleting, setDeleting] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  const [confirmUpdate, setConfirmUpdate] = useState<Spark | null>(null);
  const [updating, setUpdating] = useState<string | null>(null);
  const [updateError, setUpdateError] = useState<string | null>(null);
  const [confirmPrune, setConfirmPrune] = useState<Spark | null>(null);
  const [pruning, setPruning] = useState(false);
  const [pruneError, setPruneError] = useState<string | null>(null);
  const [pruneResult, setPruneResult] = useState<string | null>(null);
  const [waiting, setWaiting] = useState<{
    id: string;
    version: string;
  } | null>(null);
  useEffect(() => {
    if (!waiting) return;
    const controller = new AbortController();
    const deadline = Date.now() + UPDATE_RECONNECT_TIMEOUT_MS;
    let checking = false;
    const timer = window.setInterval(() => {
      if (checking) return;
      checking = true;
      void listSparks(
        AbortSignal.any([
          controller.signal,
          AbortSignal.timeout(UPDATE_STATUS_REQUEST_TIMEOUT_MS),
        ]),
      )
        .then((items) => {
          if (controller.signal.aborted) return;
          const updated = items.find((item) => item.id === waiting.id);
          if (updated?.connected && updated.agent_version === waiting.version) {
            addSpark(updated);
            setWaiting(null);
            setUpdating(null);
          } else if (Date.now() > deadline) {
            setUpdateError(
              i18n.t(
                "sparksPage.theUpdateWasAcceptedButSparkHasNotReconnectedWith",
              ),
            );
            setWaiting(null);
            setUpdating(null);
          }
        })
        .catch(() => {
          if (!controller.signal.aborted && Date.now() > deadline) {
            setUpdateError(
              i18n.t(
                "sparksPage.unableToConfirmTheUpdatedSparkConnectionRefreshToCheck",
              ),
            );
            setWaiting(null);
            setUpdating(null);
          }
        })
        .finally(() => {
          checking = false;
        });
    }, UPDATE_STATUS_POLL_INTERVAL_MS);
    return () => {
      controller.abort();
      window.clearInterval(timer);
    };
  }, [waiting, addSpark]);
  async function updateClient(spark: Spark) {
    if (updating || !spark.latest_agent_version) return;
    setUpdating(spark.id);
    setUpdateError(null);
    setConfirmUpdate(null);
    try {
      await updateSparkClient(spark.id, spark.latest_agent_version);
      setWaiting({ id: spark.id, version: spark.latest_agent_version });
    } catch (cause) {
      setUpdateError(
        cause instanceof Error
          ? cause.message
          : i18n.t("sparksPage.unableToUpdateSpark"),
      );
      setUpdating(null);
    }
  }
  async function prune(spark: Spark) {
    if (pruning) return;
    setPruning(true);
    setPruneError(null);
    setPruneResult(null);
    try {
      const result = await pruneSpark(spark.id);
      setPruneResult(t("sparksPage.pruneResult", { ...result }));
      void refresh();
    } catch (cause) {
      setPruneError(
        cause instanceof Error ? cause.message : t("sparksPage.pruneFailed"),
      );
    } finally {
      setPruning(false);
    }
  }
  async function remove(spark: Spark) {
    if (
      deleting !== null ||
      !window.confirm(
        i18n.t("sparksPage.removeAndRevokeItsCredentialsMoveOrDeleteAllIts", {
          sparkName: spark.name,
        }),
      )
    )
      return;
    setDeleting(spark.id);
    setDeleteError(null);
    try {
      await deleteSpark(spark.id);
      setDetails(null);
      await refresh();
    } catch (cause) {
      setDeleteError(
        cause instanceof Error
          ? cause.message
          : i18n.t("sparksPage.unableToRemoveSpark"),
      );
    } finally {
      setDeleting(null);
    }
  }

  function openAdd() {
    setEditing(null);
    setAdding(true);
  }

  return (
    <section className="page" aria-labelledby="page-title">
      <PageHeader
        title={t("common.sparks")}
        description={t("sparksPage.theNodesThatHostYourGameServers")}
        actions={
          <>
            <Button
              disabled={loading}
              onClick={() => {
                void refresh();
              }}
              loading={loading}
              loadingLabel={t("sparksPage.refreshing")}
            >
              {t("common.refresh")}
            </Button>
            <Button variant="primary" onClick={openAdd}>
              {t("sparksPage.addSpark")}
            </Button>
          </>
        }
      />
      {(deleteError ?? (sparks.length ? error : null)) && (
        <InlineMessage role="alert">{error ?? deleteError}</InlineMessage>
      )}
      <ContentPanel aria-busy={loading}>
        {updateError && <InlineMessage>{updateError}</InlineMessage>}
        <SparksTable
          loading={loading}
          error={error}
          onRetry={() => {
            void refresh();
          }}
          onAdd={openAdd}
          onDetails={setDetails}
          sparks={sparks}
          servers={
            monitoring.snapshot?.incomplete
              ? undefined
              : monitoring.snapshot?.servers
          }
          elapsed={monitoring.elapsed}
          updating={updating}
          onUpdate={setConfirmUpdate}
        />
      </ContentPanel>
      <p className="list-summary" role="status">
        {loading
          ? t("sparksPage.loadingSparks")
          : t("sparksPage.count", { count: sparks.length })}
      </p>
      {adding && (
        <SparkDialog
          spark={editing}
          onClose={() => setAdding(false)}
          onSaved={(spark) => {
            addSpark(spark);
            void refresh();
            if (!editing) setEnrolling({ spark, reEnrollment: false });
          }}
        />
      )}
      {enrolling && (
        <EnrollSparkDialog
          spark={
            sparks.find((spark) => spark.id === enrolling.spark.id) ??
            enrolling.spark
          }
          reEnrollment={enrolling.reEnrollment}
          onClose={() => setEnrolling(null)}
        />
      )}
      {details && (
        <SparkDetailsDialog
          key={details.id}
          id={details.id}
          spark={details}
          title={
            <>
              <span>{details.name}</span>
              <SparkConnectionState
                disabled={details.disabled}
                connected={details.connected}
                version={details.agent_version}
                updating={updating === details.id}
              />
            </>
          }
          onDelete={() => {
            void remove(details);
          }}
          deleting={deleting === details.id}
          deleteDisabled={deleting !== null || updating !== null}
          actionError={updateError ?? deleteError}
          actions={
            <>
              <Button
                variant="warning"
                disabled={
                  !details.connected ||
                  details.disabled ||
                  pruning ||
                  deleting !== null ||
                  updating !== null ||
                  !details.capabilities?.includes("spark.prune.v1")
                }
                onClick={() => {
                  setPruneError(null);
                  setPruneResult(null);
                  setConfirmPrune(details);
                }}
              >
                {t("sparksPage.prune")}
              </Button>
              {details.agent_update_available && (
                <Button
                  variant="warning"
                  disabled={
                    updating !== null ||
                    !details.capabilities?.includes("spark.self-update.v1")
                  }
                  onClick={() => setConfirmUpdate(details)}
                  loading={updating === details.id}
                  loadingLabel={t("sparksPage.updating")}
                >
                  {t("sparksPage.updateSpark")}
                </Button>
              )}
              <Button
                type="button"
                variant="warning"
                disabled={
                  details.disabled || deleting !== null || updating !== null
                }
                onClick={() => {
                  setConfirmEnrollment(details);
                }}
              >
                {t("sparksPage.reEnroll")}
              </Button>
            </>
          }
          onClose={() => setDetails(null)}
          onEdit={() => {
            setDetails(null);
            setEditing(details);
            setAdding(true);
          }}
          editDisabled={deleting !== null}
        />
      )}
      {confirmPrune && (
        <ConfirmDialog
          title={t("sparksPage.pruneTitle", { name: confirmPrune.name })}
          description={t("sparksPage.pruneDescription")}
          confirmLabel={t(pruneResult ? "common.close" : "sparksPage.prune")}
          warning
          busy={pruning}
          error={pruneError}
          onClose={() => {
            if (!pruning) setConfirmPrune(null);
          }}
          onConfirm={() => {
            if (pruneResult) setConfirmPrune(null);
            else void prune(confirmPrune);
          }}
        >
          {pruneResult && (
            <InlineMessage tone="success">{pruneResult}</InlineMessage>
          )}
        </ConfirmDialog>
      )}
      {confirmEnrollment && (
        <ConfirmDialog
          title={t("sparksPage.reEnroll2", {
            confirmEnrollmentName: confirmEnrollment.name,
          })}
          description={t(
            "sparksPage.runningTheReEnrollmentCommandWillReplaceThisSparkS",
          )}
          confirmLabel={t("sparksPage.continueToReEnrollment")}
          warning
          busy={false}
          error={null}
          onClose={() => setConfirmEnrollment(null)}
          onConfirm={() => {
            setEnrolling({ spark: confirmEnrollment, reEnrollment: true });
            setConfirmEnrollment(null);
            setDetails(null);
          }}
        >
          <p>
            {t("sparksPage.openingTheHelperOrGeneratingACommandDoesNotReset")}
          </p>
        </ConfirmDialog>
      )}
      {confirmUpdate && (
        <ConfirmDialog
          title={t("sparksPage.update", {
            confirmUpdateName: confirmUpdate.name,
          })}
          description={t(
            "sparksPage.installSparkFromThisCampfirePanelTheNodeBrieflyDisconnects",
            {
              confirmUpdateLatestAgent:
                confirmUpdate.latest_agent_version ??
                t("sparksPage.matchingThisPanel"),
            },
          )}
          confirmLabel={t("sparksPage.updateSpark")}
          warning
          busy={false}
          error={null}
          onClose={() => setConfirmUpdate(null)}
          onConfirm={() => {
            void updateClient(confirmUpdate);
          }}
        />
      )}
    </section>
  );
}
