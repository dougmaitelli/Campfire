import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import type { TableSelection } from "./useTableSelection";
import { interactiveSelector, isInteractionTarget } from "./interactionTargets";
import "./table-interactions.css";

/** Mouse-only, opt-in surface. Touch keeps native scrolling and checkboxes. */
export function MarqueeSelection({
  selection,
  children,
}: {
  selection: TableSelection;
  children: ReactNode;
}) {
  const cancel = useRef<(() => void) | null>(null);
  const suppressClick = useRef(false);
  const [rectangle, setRectangle] = useState<CSSProperties | null>(null);
  const rowKey = JSON.stringify(selection.ids);
  useEffect(() => {
    cancel.current?.();
    return () => {
      cancel.current?.();
    };
  }, [selection.scope, selection.disabled, rowKey]);

  return (
    <div
      className="table-selection-surface"
      onClickCapture={(event) => {
        if (!suppressClick.current) return;
        suppressClick.current = false;
        event.preventDefault();
        event.stopPropagation();
      }}
      onPointerDown={(event) => {
        suppressClick.current = false;
        if (
          selection.disabled ||
          event.button !== 0 ||
          event.pointerType !== "mouse" ||
          event.altKey
        )
          return;
        if (
          isInteractionTarget(
            event.target,
            event.currentTarget,
            `${interactiveSelector}, [draggable=true], thead`,
          )
        )
          return;
        const root = event.currentTarget;
        cancel.current?.();
        const bounds = root.getBoundingClientRect();
        const start = {
          x: event.clientX - bounds.left + root.scrollLeft,
          y: event.clientY - bounds.top + root.scrollTop,
        };
        const initial = [...selection.selectedIDs];
        const additive = event.ctrlKey || event.metaKey || event.shiftKey;
        const pointerID = event.pointerId;
        const origin = { x: event.clientX, y: event.clientY };
        let point = origin;
        let active = false;
        let frame = 0;
        let lastSelection = "";
        let scroller: HTMLElement | null = root;
        while (
          scroller &&
          !(
            scroller.scrollHeight > scroller.clientHeight &&
            /auto|scroll/.test(getComputedStyle(scroller).overflowY)
          )
        ) {
          scroller = scroller.parentElement;
        }
        const scrollElement = scroller ?? document.documentElement;
        function update() {
          const box = root.getBoundingClientRect();
          const x = Math.max(
            0,
            Math.min(root.scrollWidth, point.x - box.left + root.scrollLeft),
          );
          const y = Math.max(
            0,
            Math.min(root.scrollHeight, point.y - box.top + root.scrollTop),
          );
          const left = Math.min(start.x, x);
          const top = Math.min(start.y, y);
          const width = Math.abs(start.x - x);
          const height = Math.abs(start.y - y);
          setRectangle({ left, top, width, height });
          const ids = new Set(additive ? initial : []);
          for (const row of root.querySelectorAll<HTMLElement>(
            "[data-selection-id]",
          )) {
            const rect = row.getBoundingClientRect();
            if (
              rect.right > box.left + left - root.scrollLeft &&
              rect.left < box.left + left + width - root.scrollLeft &&
              rect.bottom > box.top + top - root.scrollTop &&
              rect.top < box.top + top + height - root.scrollTop
            ) {
              const id = row.dataset.selectionId;
              if (id !== undefined) ids.add(id);
            }
          }
          const key = JSON.stringify([...ids]);
          if (key !== lastSelection) {
            lastSelection = key;
            selection.setSelection([...ids]);
          }
        }
        function autoScroll() {
          if (!active) return;
          const clip = scroller?.getBoundingClientRect();
          const top = Math.max(0, clip?.top ?? 0);
          const bottom = Math.min(
            window.innerHeight,
            clip?.bottom ?? window.innerHeight,
          );
          const delta =
            point.y < top + 28 ? -12 : point.y > bottom - 28 ? 12 : 0;
          const before = scrollElement.scrollTop;
          if (delta) scrollElement.scrollTop += delta;
          if (scrollElement.scrollTop !== before) update();
          frame = requestAnimationFrame(autoScroll);
        }
        function move(next: PointerEvent) {
          if (next.pointerId !== pointerID) return;
          point = { x: next.clientX, y: next.clientY };
          if (!active && Math.hypot(point.x - origin.x, point.y - origin.y) < 5)
            return;
          if (!active) {
            active = true;
            root.setPointerCapture(pointerID);
            frame = requestAnimationFrame(autoScroll);
          }
          next.preventDefault();
          window.getSelection()?.removeAllRanges();
          update();
        }
        function finish(restore = false) {
          if (restore && active) selection.setSelection(initial);
          if (active) {
            suppressClick.current = true;
            window.setTimeout(() => {
              suppressClick.current = false;
            }, 0);
          }
          cancelAnimationFrame(frame);
          window.removeEventListener("pointermove", move);
          window.removeEventListener("pointerup", up);
          window.removeEventListener("pointercancel", cancelPointer);
          window.removeEventListener("lostpointercapture", cancelPointer);
          window.removeEventListener("keydown", key, true);
          window.removeEventListener("blur", aborted);
          window.removeEventListener("dragstart", aborted);
          document.removeEventListener("scroll", scroll, true);
          if (root.hasPointerCapture(pointerID))
            root.releasePointerCapture(pointerID);
          cancel.current = null;
          setRectangle(null);
        }
        function up(next: PointerEvent) {
          if (next.pointerId === pointerID) finish();
        }
        function cancelPointer(next: PointerEvent) {
          if (next.pointerId === pointerID) finish(true);
        }
        function aborted() {
          finish(true);
        }
        function key(next: KeyboardEvent) {
          if (next.key === "Escape") {
            next.preventDefault();
            next.stopPropagation();
            finish(true);
          }
        }
        function scroll() {
          if (active) update();
        }
        cancel.current = () => finish();
        window.addEventListener("pointermove", move, { passive: false });
        window.addEventListener("pointerup", up);
        window.addEventListener("pointercancel", cancelPointer);
        window.addEventListener("lostpointercapture", cancelPointer);
        window.addEventListener("keydown", key, true);
        window.addEventListener("blur", aborted);
        window.addEventListener("dragstart", aborted);
        document.addEventListener("scroll", scroll, true);
      }}
    >
      {children}
      {rectangle && (
        <div
          className="table-selection-marquee"
          style={rectangle}
          aria-hidden="true"
        />
      )}
    </div>
  );
}
