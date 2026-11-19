import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type HTMLAttributes,
} from "react";
import { hasExternalFiles } from "../dragDrop/dataTransfer";
import { isInteractionTarget } from "./interactionTargets";
import "./table-interactions.css";

const itemType = "application/x-campfire-table-items";
interface DragSession {
  token: string;
  scope: string;
  ids: string[];
}

/** One controller can attach sources and targets to tables, breadcrumbs, or other surfaces. */
export function useTableDragDrop({
  scope,
  disabled = false,
  getItems,
  canDrop,
  onDrop,
  onDragItems,
}: {
  scope: string;
  disabled?: boolean;
  getItems: (id: string) => readonly string[];
  canDrop: (ids: readonly string[], target: string) => boolean;
  onDrop: (ids: readonly string[], target: string) => void;
  onDragItems?: (ids: string[]) => void;
}) {
  const session = useRef<DragSession | null>(null);
  const origin = useRef<{
    source: HTMLElement;
    target: EventTarget | null;
  } | null>(null);
  const [draggedIDs, setDraggedIDs] = useState<readonly string[]>([]);
  const [target, setTarget] = useState<string | null>(null);
  const [context, setContext] = useState({ scope, disabled });
  if (context.scope !== scope || context.disabled !== disabled) {
    setContext({ scope, disabled });
    setDraggedIDs([]);
    setTarget(null);
  }
  const reset = useCallback(() => {
    session.current = null;
    origin.current = null;
    setDraggedIDs([]);
    setTarget(null);
  }, []);
  useEffect(() => {
    function releasePointer() {
      origin.current = null;
    }
    function escape(event: KeyboardEvent) {
      if (event.key === "Escape") reset();
    }
    window.addEventListener("pointerup", releasePointer);
    window.addEventListener("dragend", reset);
    window.addEventListener("drop", reset);
    window.addEventListener("blur", reset);
    window.addEventListener("keydown", escape);
    return () => {
      session.current = null;
      origin.current = null;
      window.removeEventListener("pointerup", releasePointer);
      window.removeEventListener("dragend", reset);
      window.removeEventListener("drop", reset);
      window.removeEventListener("blur", reset);
      window.removeEventListener("keydown", escape);
    };
  }, [reset]);
  useEffect(() => {
    session.current = null;
    origin.current = null;
  }, [scope, disabled]);

  function sourceProps(
    id: string,
    enabled = true,
  ): HTMLAttributes<HTMLElement> {
    return {
      draggable: !disabled && enabled,
      onPointerDownCapture: (event) => {
        // dragstart targets the draggable ancestor, not necessarily the control
        // where the gesture began. The innermost explicit source owns the origin.
        origin.current = { source: event.currentTarget, target: event.target };
      },
      onDragStart: (event) => {
        const pointer = origin.current;
        const target =
          pointer && event.currentTarget.contains(pointer.source)
            ? pointer.target
            : event.target;
        if (
          event.defaultPrevented ||
          disabled ||
          !enabled ||
          isInteractionTarget(target, event.currentTarget)
        ) {
          event.preventDefault();
          return;
        }
        const ids = [...new Set(getItems(id))];
        if (!ids.length) {
          event.preventDefault();
          return;
        }
        event.stopPropagation();
        const token = crypto.randomUUID();
        session.current = { scope, token, ids };
        event.dataTransfer.clearData();
        event.dataTransfer.setData(itemType, token);
        event.dataTransfer.effectAllowed = "move";
        setDraggedIDs(ids);
        onDragItems?.(ids);
      },
      onDragEnd: reset,
    };
  }
  function current(event: DragEvent<HTMLElement>) {
    const active = session.current;
    if (
      !active ||
      active.scope !== scope ||
      hasExternalFiles(event.dataTransfer) ||
      !event.dataTransfer.types.includes(itemType)
    )
      return null;
    return active;
  }
  function targetProps(id: string): HTMLAttributes<HTMLElement> & {
    "data-drop-target"?: boolean | undefined;
  } {
    function over(event: DragEvent<HTMLElement>) {
      const active = current(event);
      if (!active) return;
      event.preventDefault();
      event.stopPropagation();
      const allowed = !disabled && canDrop(active.ids, id);
      event.dataTransfer.dropEffect = allowed ? "move" : "none";
      setTarget(allowed ? id : null);
    }
    return {
      "data-drop-target": target === id || undefined,
      onDragEnter: over,
      onDragOver: over,
      onDragLeave: (event) => {
        if (
          event.relatedTarget instanceof Node &&
          event.currentTarget.contains(event.relatedTarget)
        )
          return;
        setTarget((value) => (value === id ? null : value));
      },
      onDrop: (event) => {
        const active = current(event);
        if (!active) return;
        event.preventDefault();
        event.stopPropagation();
        const allowed =
          !disabled &&
          event.dataTransfer.getData(itemType) === active.token &&
          canDrop(active.ids, id);
        reset();
        if (allowed) onDrop(active.ids, id);
      },
    };
  }
  return { sourceProps, targetProps, draggedIDs };
}
