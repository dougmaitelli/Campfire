import { useTranslation } from "react-i18next";
import { bytes } from "../../api/monitoring";
import { Button } from "../forms/Button";
import "./file-upload.css";

export interface UploadProgress {
  name: string;
  index: number;
  count: number;
  completedFiles: number;
  uploadedBytes: number;
  totalBytes: number;
  phase: "uploading" | "saving" | "cancelling";
}

export function FileUploadProgress({
  progress,
  onCancel,
}: {
  progress: UploadProgress;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const percent = Math.min(
    100,
    Math.floor(
      100 *
        (progress.totalBytes > 0
          ? progress.uploadedBytes / progress.totalBytes
          : progress.completedFiles / progress.count),
    ),
  );
  const transferred = t("fileUploads.transferred", {
    uploaded: bytes(progress.uploadedBytes),
    total: bytes(progress.totalBytes),
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
          disabled={progress.phase === "cancelling"}
          onClick={onCancel}
        >
          {t("fileUploads.cancelUpload")}
        </Button>
      </div>
      <progress
        className="file-upload-bar"
        max={100}
        value={percent}
        aria-label={t("fileUploads.uploadProgress")}
        aria-valuetext={transferred}
      />
      <div className="file-upload-metrics">
        <span>{transferred}</span>
        <strong>{percent}%</strong>
      </div>
    </section>
  );
}
