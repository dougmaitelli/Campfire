import {
  CompressionState,
  type ExtractionState,
  type ExtractionOutcomeStatus,
  type ExtractionAction,
  type BrowserPublicationStatus,
  type FileKind,
  type FileOperation,
} from "../contracts/domains";
import type { WorkProgress } from "./workProgress";

export interface FileEntry {
  mode?: number;
  name: string;
  kind: FileKind;
  size: number;
  modified_at: string;
}
export {
  ExtractionAction,
  ExtractionState,
  ExtractionOutcomeStatus,
} from "../contracts/domains";

export interface ExtractionChoice {
  action: ExtractionAction;
  name?: string;
}
export interface ExtractionConflict {
  id: string;
  path: string;
  destination: string;
  kind: FileKind;
  size: number;
  existing?: FileEntry;
  changed?: boolean;
  protected?: boolean;
}
export type PublicationStatus =
  | Exclude<ExtractionOutcomeStatus, ExtractionOutcomeStatus.Pending>
  | BrowserPublicationStatus;
export interface ExtractionOutcome {
  path: string;
  destination: string;
  status: ExtractionOutcomeStatus;
  message?: string;
}
export interface ExtractionStatus {
  work_progress?: WorkProgress;
  id: string;
  version: number;
  state: ExtractionState;
  expires_at: string;
  conflict?: ExtractionConflict;
  pending_conflicts: number;
  remaining_entries: number;
  outcomes: ExtractionOutcome[];
  message?: string;
}
const compressionTerminal: Record<CompressionState, boolean> = {
  [CompressionState.Running]: false,
  [CompressionState.Cancelling]: false,
  [CompressionState.Completed]: true,
  [CompressionState.Cancelled]: true,
  [CompressionState.Failed]: true,
};
export const isCompressionTerminal = (state: CompressionState): boolean =>
  compressionTerminal[state];

export interface CompressionStatus {
  id: string;
  state: CompressionState;
  message?: string;
  work_progress?: WorkProgress;
}
export interface FileResult {
  server_id: string;
  path: string;
  entries?: FileEntry[];
  data?: string;
  revision?: string;
  upload_id?: string;
  compression?: CompressionStatus;
  extraction?: ExtractionStatus;
}
export interface FileRequest {
  operation: FileOperation;
  path: string;
  mode?: number;
  paths?: string[];
  destination?: string;
  data?: string;
  revision?: string;
  upload_id?: string;
  offset?: number;
  size?: number;
  compression?: { id: string };
  extraction?: {
    id: string;
    fresh?: boolean;
    conflict_id?: string;
    action?: ExtractionChoice["action"];
    name?: string;
  };
}
