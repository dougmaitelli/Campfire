import { useEffect, useRef, useState, type ComponentProps } from "react";
import "./table-interactions.css";

import { isInteractionTarget } from "./interactionTargets";

export interface SelectionGesture {
  shift?: boolean;
  additive?: boolean;
  checked?: boolean;
}

/** Selection is scoped to the supplied, ordered rows, never unseen pages. */
export function useTableSelection({
  ids,
  selectedIDs,
  onSelectionChange,
  scope,
  disabled = false,
  getText = (id) => id,
  onActivate,
}: {
  ids: readonly string[];
  selectedIDs: readonly string[];
  onSelectionChange: (ids: string[]) => void;
  scope: string;
  disabled?: boolean;
  getText?: (id: string) => string;
  onActivate?: (id: string) => void;
}) {
  const anchor = useRef<{ scope: string; id: string } | null>(null);
  const previousScope = useRef(scope);
  const rows = useRef(new Map<string, HTMLTableRowElement>());
  const search = useRef({ text: "", time: 0 });
  const [focus, setFocus] = useState<{ scope: string; id: string } | null>(
    null,
  );
  const valid = new Set(ids);
  const selected = new Set(selectedIDs.filter((id) => valid.has(id)));
  const focusedID =
    focus?.scope === scope && valid.has(focus.id) ? focus.id : ids[0];

  useEffect(() => {
    if (previousScope.current !== scope) {
      previousScope.current = scope;
      anchor.current = null;
      search.current = { text: "", time: 0 };
      if (selectedIDs.length) onSelectionChange([]);
    } else if (selectedIDs.some((id) => !ids.includes(id))) {
      onSelectionChange(selectedIDs.filter((id) => ids.includes(id)));
    }
  }, [scope, ids, selectedIDs, onSelectionChange]);

  function setSelection(next: readonly string[]) {
    if (!disabled)
      onSelectionChange([...new Set(next)].filter((id) => valid.has(id)));
  }
  function clear() {
    anchor.current = null;
    search.current = { text: "", time: 0 };
    setSelection([]);
  }
  function select(id: string, gesture: SelectionGesture = {}) {
    if (disabled || !valid.has(id)) return;
    const start =
      anchor.current?.scope === scope ? ids.indexOf(anchor.current.id) : -1;
    if (gesture.shift && start >= 0) {
      const end = ids.indexOf(id);
      const range = ids.slice(Math.min(start, end), Math.max(start, end) + 1);
      const next = new Set(gesture.additive ? selected : []);
      for (const item of range) {
        if (gesture.checked === false) next.delete(item);
        else next.add(item);
      }
      setSelection([...next]);
    } else {
      const next = new Set(gesture.additive ? selected : []);
      const checked =
        gesture.checked ?? (!gesture.additive || !selected.has(id));
      if (checked) next.add(id);
      else next.delete(id);
      anchor.current = { scope, id };
      setSelection([...next]);
    }
    setFocus({ scope, id });
  }
  function focusRow(id: string) {
    setFocus({ scope, id });
    rows.current.get(id)?.focus();
  }
  function activationProps(
    id: string,
  ): Pick<ComponentProps<"button">, "onClick"> {
    return {
      onClick: (event) => {
        if (disabled || event.defaultPrevented) return;
        if (event.shiftKey || event.ctrlKey || event.metaKey) {
          event.preventDefault();
          select(id, {
            shift: event.shiftKey,
            additive: event.ctrlKey || event.metaKey,
          });
          focusRow(id);
        } else onActivate?.(id);
      },
    };
  }
  function rowProps(
    id: string,
  ): ComponentProps<"tr"> & { "data-selection-id"?: string } {
    if (!valid.has(id)) return {};
    return {
      ref: (node) => {
        if (node) rows.current.set(id, node);
        else rows.current.delete(id);
      },
      "data-selection-id": id,
      "aria-selected": selected.has(id),
      tabIndex: !disabled && focusedID === id ? 0 : -1,
      onFocus: (event) => {
        if (event.target === event.currentTarget) setFocus({ scope, id });
      },
      onClick: (event) => {
        if (
          disabled ||
          event.defaultPrevented ||
          isInteractionTarget(event.target, event.currentTarget)
        )
          return;
        event.preventDefault(); // Selection consumes the click, not row activation.
        select(id, {
          shift: event.shiftKey,
          additive: event.ctrlKey || event.metaKey,
        });
        focusRow(id);
      },
      onKeyDown: (event) => {
        if (
          disabled ||
          event.defaultPrevented ||
          event.altKey ||
          event.nativeEvent.isComposing ||
          event.target !== event.currentTarget
        )
          return;
        const additive = event.ctrlKey || event.metaKey;
        const index = ids.indexOf(id);
        const range =
          event.shiftKey &&
          ["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key);
        if (event.key.length !== 1 || additive)
          search.current = { text: "", time: 0 };
        let next: string | undefined;
        switch (event.key) {
          case "ArrowDown":
            next = ids[Math.min(ids.length - 1, index + 1)];
            break;
          case "ArrowUp":
            next = ids[Math.max(0, index - 1)];
            break;
          case "Home":
            next = ids[0];
            break;
          case "End":
            next = ids[ids.length - 1];
            break;
          case " ":
            event.preventDefault();
            select(id, { shift: event.shiftKey, additive: true });
            return;
          case "Escape":
            event.preventDefault();
            clear();
            return;
          case "Enter":
            if (onActivate) {
              event.preventDefault();
              onActivate(id);
            }
            return;
          default:
            if (additive && event.key.toLowerCase() === "a") {
              event.preventDefault();
              setSelection(ids);
              return;
            }
            if (!additive && event.key.length === 1) {
              const now = performance.now();
              const text =
                (now - search.current.time < 700 ? search.current.text : "") +
                event.key.toLocaleLowerCase();
              search.current = { text, time: now };
              // Repeated letters cycle; multi-letter prefixes find a name.
              const prefix = [...text].every((letter) => letter === text[0])
                ? text.charAt(0)
                : text;
              const ordered = [
                ...ids.slice(index + (prefix.length === 1 ? 1 : 0)),
                ...ids.slice(0, index + 1),
              ];
              next = ordered.find((item) =>
                getText(item).toLocaleLowerCase().startsWith(prefix),
              );
            }
        }
        if (next) {
          event.preventDefault();
          if (range && !anchor.current) anchor.current = { scope, id };
          if (!additive || range) select(next, { shift: range, additive });
          focusRow(next);
        }
      },
    };
  }
  return {
    ids,
    selectedIDs: [...selected],
    scope,
    disabled,
    select,
    setSelection,
    clear,
    rowProps,
    activationProps,
    isSelected: (id: string) => selected.has(id),
    isSelectable: (id: string) => valid.has(id),
  };
}

export type TableSelection = ReturnType<typeof useTableSelection>;
