import {
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import { NavigationIcon } from "../navigation/NavigationIcon";
import "./file-upload.css";

function hasFiles(transfer: DataTransfer | null) {
  return transfer?.types.includes("Files") === true;
}

export function FileDropZone({
  enabled,
  directory,
  onFiles,
  onError,
  children,
}: {
  enabled: boolean;
  directory: string;
  onFiles: (files: File[]) => void;
  onError: (message: string) => void;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const [dragging, setDragging] = useState(false);
  const depth = useRef(0);
  function reset() {
    depth.current = 0;
    setDragging(false);
  }
  useEffect(() => {
    function preventNavigation(event: globalThis.DragEvent) {
      if (hasFiles(event.dataTransfer) && !event.defaultPrevented) {
        event.preventDefault();
        if (event.dataTransfer) event.dataTransfer.dropEffect = "none";
      }
      if (event.type === "drop") reset();
    }
    function leaveWindow(event: globalThis.DragEvent) {
      if (
        event.relatedTarget === null &&
        (event.target === document.documentElement ||
          event.target === document.body)
      )
        reset();
    }
    function escape(event: KeyboardEvent) {
      if (event.key === "Escape") reset();
    }
    window.addEventListener("dragover", preventNavigation);
    window.addEventListener("drop", preventNavigation);
    window.addEventListener("dragleave", leaveWindow);
    window.addEventListener("dragend", reset);
    window.addEventListener("blur", reset);
    window.addEventListener("keydown", escape);
    return () => {
      window.removeEventListener("dragover", preventNavigation);
      window.removeEventListener("drop", preventNavigation);
      window.removeEventListener("dragleave", leaveWindow);
      window.removeEventListener("dragend", reset);
      window.removeEventListener("blur", reset);
      window.removeEventListener("keydown", escape);
    };
  }, []);
  function over(event: DragEvent<HTMLDivElement>) {
    if (!hasFiles(event.dataTransfer)) return;
    event.preventDefault();
    event.dataTransfer.dropEffect = enabled ? "copy" : "none";
    if (enabled) setDragging(true);
  }
  return (
    <div
      className="drawer-content file-drop-zone"
      role="region"
      aria-label={t("fileUploads.fileBrowser")}
      onDragEnter={(event) => {
        if (!hasFiles(event.dataTransfer)) return;
        event.preventDefault();
        depth.current += 1;
        if (enabled) setDragging(true);
      }}
      onDragOver={over}
      onDragLeave={(event) => {
        if (!hasFiles(event.dataTransfer)) return;
        depth.current = Math.max(0, depth.current - 1);
        if (depth.current === 0) setDragging(false);
      }}
      onDrop={(event) => {
        if (!hasFiles(event.dataTransfer)) return;
        event.preventDefault();
        event.stopPropagation();
        reset();
        if (!enabled) return;
        if (
          Array.from(event.dataTransfer.items).some(
            (item) => item.webkitGetAsEntry()?.isDirectory,
          )
        ) {
          onError(t("fileUploads.foldersUnsupported"));
          return;
        }
        const files = Array.from(event.dataTransfer.files);
        if (files.length) onFiles(files);
      }}
    >
      {children}
      {dragging && enabled && (
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
