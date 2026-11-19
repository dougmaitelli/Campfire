import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { useExternalFileDrop } from "../dragDrop/useExternalFileDrop";
import "./file-upload.css";

/** File-browser presentation and directory-upload policy over a reusable drop surface. */
export function FileDropZone({
  className,
  enabled,
  directory,
  onFiles,
  onError,
  children,
}: {
  className?: string;
  enabled: boolean;
  directory: string;
  onFiles: (files: File[]) => void;
  onError: (message: string) => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const { dragging, dropProps } = useExternalFileDrop({
    enabled,
    onDrop: ({ files, hasDirectories }) => {
      if (hasDirectories) onError(t("fileUploads.foldersUnsupported"));
      else if (files.length) onFiles(files);
    },
  });
  return (
    <div
      {...dropProps}
      className={["file-drop-zone", className].filter(Boolean).join(" ")}
      role="region"
      aria-label={t("fileUploads.fileBrowser")}
    >
      {children}
      {dragging && (
        <div className="file-drop-overlay" role="status">
          <div className="file-drop-message">
            <NavigationIcon name="upload" />
            <strong>{t("fileUploads.releaseToUpload")}</strong>
            <span>{t("fileUploads.destination", { directory })}</span>
          </div>
        </div>
      )}
    </div>
  );
}
