import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type HTMLAttributes,
} from "react";
import { hasExternalFiles } from "./dataTransfer";

export interface ExternalFileDrop {
  files: File[];
  hasDirectories: boolean;
}

/** Attach to any surface. File policy and upload execution belong to the consumer. */
export function useExternalFileDrop({
  enabled,
  onDrop,
}: {
  enabled: boolean;
  onDrop: (drop: ExternalFileDrop) => void;
}) {
  const [dragging, setDragging] = useState(false);
  const depth = useRef(0);
  const [previousEnabled, setPreviousEnabled] = useState(enabled);
  if (previousEnabled !== enabled) {
    setPreviousEnabled(enabled);
    setDragging(false);
  }
  const reset = useCallback(() => {
    depth.current = 0;
    setDragging(false);
  }, []);
  useEffect(() => {
    function preventNavigation(event: globalThis.DragEvent) {
      if (hasExternalFiles(event.dataTransfer) && !event.defaultPrevented) {
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
  }, [reset]);
  useEffect(() => {
    depth.current = 0;
  }, [enabled]);
  function over(event: DragEvent<HTMLElement>) {
    if (!hasExternalFiles(event.dataTransfer)) return;
    event.preventDefault();
    event.stopPropagation();
    event.dataTransfer.dropEffect = enabled ? "copy" : "none";
    if (enabled) setDragging(true);
  }
  const dropProps: HTMLAttributes<HTMLElement> = {
    onDragEnter: (event) => {
      if (!hasExternalFiles(event.dataTransfer)) return;
      event.preventDefault();
      event.stopPropagation();
      depth.current += 1;
      if (enabled) setDragging(true);
    },
    onDragOver: over,
    onDragLeave: (event) => {
      if (!hasExternalFiles(event.dataTransfer)) return;
      event.stopPropagation();
      depth.current = Math.max(0, depth.current - 1);
      if (!depth.current) setDragging(false);
    },
    onDrop: (event) => {
      if (!hasExternalFiles(event.dataTransfer)) return;
      event.preventDefault();
      event.stopPropagation();
      reset();
      if (!enabled) return;
      onDrop({
        files: Array.from(event.dataTransfer.files),
        hasDirectories: Array.from(event.dataTransfer.items).some(
          (item) => item.webkitGetAsEntry()?.isDirectory === true,
        ),
      });
    },
  };
  return { dragging: dragging && enabled, dropProps };
}
