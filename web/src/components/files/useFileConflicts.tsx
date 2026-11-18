import { useEffect, useRef, useState } from "react";
import { availableFileName } from "./fileNames";
import {
  conflictActions,
  type FileConflict,
  type FileConflictChoice,
  type ResolveFileConflict,
} from "./fileConflicts";
import { FileConflictDialog } from "./FileConflictDialog";

// Both entry points get the same decisions, bulk-action rules, name validation,
// cancellation and dialog. A session's choices never leak to another operation.
export function useFileConflicts() {
  const [pending, setPending] = useState<{
    id: number;
    operation: "upload" | "extract";
    conflict: FileConflict;
    reserved: ReadonlySet<string>;
    hasRemainingConflicts: boolean;
  } | null>(null);
  const sequence = useRef(0);
  const alive = useRef(true);
  const resolver = useRef<((choice: FileConflictChoice | null) => void) | null>(
    null,
  );
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      resolver.current?.(null);
      resolver.current = null;
    };
  }, []);

  function finish(choice: FileConflictChoice | null) {
    const resolve = resolver.current;
    resolver.current = null;
    setPending(null);
    resolve?.(choice);
  }

  function createSession(operation: "upload" | "extract"): ResolveFileConflict {
    const bulk: FileConflictChoice[] = [];
    return async (
      input,
      { reserved, hasRemainingConflicts = false, forcePrompt = false, signal },
    ) => {
      if (!alive.current || resolver.current || signal?.aborted) return null;
      const base = input.name.split("/").filter(Boolean).at(-1) ?? "extracted";
      const conflict = {
        ...input,
        suggestedName: availableFileName(base, reserved),
      };
      const actions = conflictActions(conflict);
      const previous = bulk.find((choice) => actions.includes(choice.action));
      if (previous && !forcePrompt)
        return { ...previous, name: conflict.suggestedName };
      const choice = await new Promise<FileConflictChoice | null>((resolve) => {
        const abort = () => finish(null);
        resolver.current = (value) => {
          signal?.removeEventListener("abort", abort);
          resolve(value);
        };
        signal?.addEventListener("abort", abort, { once: true });
        setPending({
          id: ++sequence.current,
          operation,
          conflict,
          reserved: new Set(reserved),
          hasRemainingConflicts,
        });
      });
      if (choice?.applyToAll) bulk.unshift(choice);
      return choice;
    };
  }

  return {
    createSession,
    dialog: pending ? (
      <FileConflictDialog
        key={pending.id}
        operation={pending.operation}
        conflict={pending.conflict}
        reserved={pending.reserved}
        hasRemainingConflicts={pending.hasRemainingConflicts}
        onResolve={finish}
      />
    ) : null,
  };
}
