import { FileUploadPhase } from "../../contracts/domains";
import { useTranslation } from "react-i18next";
import { formatBytes } from "../../utils/formatBytes";
import { Button } from "../forms/Button";
import { OperationProgress } from "../status/OperationProgress";
import type { UploadProgress } from "./uploadTypes";
import "./file-upload.css";

export function FileUploadProgress({
  progress,
  onCancel,
}: {
  progress: UploadProgress;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const transferred =
    progress.totalBytes > 0
      ? t("fileUploads.transferred", {
          uploaded: formatBytes(progress.uploadedBytes),
          total: formatBytes(progress.totalBytes),
        })
      : t("fileUploads.filesCompleted", {
          completed: progress.completedFiles,
          total: progress.count,
        });
  return (
    <section
      className="file-upload"
      aria-label={t("fileUploads.uploadProgress")}
    >
      <div className="file-upload-heading">
        <div className="file-upload-details">
          <strong className="file-upload-name" title={progress.name}>
            {progress.name}
          </strong>
          <span role="status">
            {t(`fileUploads.${progress.phase}`, {
              index: progress.index,
              count: progress.count,
            })}
          </span>
        </div>
        <Button
          size="small"
          disabled={progress.phase === FileUploadPhase.Cancelling}
          onClick={onCancel}
        >
          {t("fileUploads.cancelUpload")}
        </Button>
      </div>
      <OperationProgress
        label={t("fileUploads.uploadProgress")}
        value={
          progress.totalBytes > 0
            ? progress.uploadedBytes
            : progress.completedFiles
        }
        total={progress.totalBytes > 0 ? progress.totalBytes : progress.count}
        detail={transferred}
      />
    </section>
  );
}
