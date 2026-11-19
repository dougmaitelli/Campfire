import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { WorkPhase } from "../../contracts/domains";
import type { WorkProgress as Measurement } from "../../api/workProgress";
import { formatBytes } from "../../utils/formatBytes";
import { OperationProgress } from "./OperationProgress";
import "./work-progress.css";

/** The owning operation renders completion/errors. This view shows only work. */
export function WorkProgress({
  progress,
  operationID,
}: {
  progress: Measurement;
  operationID?: string | undefined;
}) {
  const { t } = useTranslation();
  // Remote clocks are not comparable with the browser clock. Repeated polls of
  // the same measurement must not renew its lifetime, either. Start a local
  // deadline when a distinct measurement (or operation) is first observed.
  const measurement = JSON.stringify([
    operationID,
    progress.phase,
    progress.updated_at,
    progress.input_bytes,
    progress.output_bytes,
    progress.input_total ?? 0,
    progress.entries,
  ]);
  const [staleMeasurement, setStaleMeasurement] = useState<string>();
  useEffect(() => {
    const timer = setTimeout(() => setStaleMeasurement(measurement), 30000);
    return () => clearTimeout(timer);
  }, [measurement]);
  const stale = staleMeasurement === measurement;
  const phase = t(`workProgress.phase.${progress.phase}`);
  const measured = [
    WorkPhase.Archiving,
    WorkPhase.Extracting,
    WorkPhase.Validating,
    WorkPhase.Uploading,
    WorkPhase.Downloading,
    WorkPhase.Restoring,
  ].some((value) => value === progress.phase);
  const input = t(
    `workProgress.input.${progress.phase === WorkPhase.Archiving || progress.phase === WorkPhase.Restoring ? "source" : progress.phase === WorkPhase.Uploading ? "storage" : "archive"}`,
  );
  const total = progress.input_total ?? 0;
  return (
    <span className="work-progress" role="group" aria-label={phase}>
      <strong>{phase}</strong>
      {measured ? (
        <OperationProgress
          label={input}
          value={progress.input_bytes}
          total={total}
          stale={stale}
          detail={
            <>
              {input}: {formatBytes(progress.input_bytes)}
              {total > 0 && <> / {formatBytes(total)}</>}
            </>
          }
        />
      ) : (
        stale && <span>{t("workProgress.stale")}</span>
      )}
      {(progress.phase === WorkPhase.Archiving ||
        progress.phase === WorkPhase.Extracting) && (
        <span>
          {t(
            progress.phase === WorkPhase.Archiving
              ? "workProgress.compressed"
              : "workProgress.decoded",
          )}
          : {formatBytes(progress.output_bytes)} ·{" "}
          {t("workProgress.entries", { count: progress.entries })}
        </span>
      )}
    </span>
  );
}
