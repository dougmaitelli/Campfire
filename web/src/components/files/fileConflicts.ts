import type { ExtractionAction, FileEntry } from "../../api/fileTypes";
import { FileKind } from "../../contracts/domains";

export type FileConflictAction = ExtractionAction;
export interface FileConflictChoice {
  action: FileConflictAction;
  name: string;
  applyToAll: boolean;
}
export interface FileConflict {
  name: string;
  protected?: boolean;
  changed?: boolean;
  message?: string;
  expiresAt?: string;
  kind?: FileKind;
  size: number;
  existing: FileEntry | null;
  duplicate: boolean;
  suggestedName: string;
}

export type ResolveFileConflict = (
  conflict: Omit<FileConflict, "suggestedName">,
  options: {
    reserved: ReadonlySet<string>;
    hasRemainingConflicts?: boolean;
    forcePrompt?: boolean;
    signal?: AbortSignal;
  },
) => Promise<FileConflictChoice | null>;

export function conflictActions(conflict: FileConflict): FileConflictAction[] {
  const actions: FileConflictAction[] = ["skip", "rename"];
  if (conflict.protected) return actions;
  if (!conflict.existing && !conflict.duplicate && conflict.changed)
    actions.splice(1, 0, "extract");
  if (
    (conflict.kind ?? FileKind.File) === FileKind.File &&
    conflict.existing?.kind === FileKind.File
  )
    actions.splice(1, 0, "overwrite");
  if (
    conflict.kind === FileKind.Directory &&
    conflict.existing?.kind === FileKind.Directory
  )
    actions.splice(1, 0, "merge");
  return actions;
}
