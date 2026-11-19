import {
  BrowserPublicationStatus,
  ExtractionOutcomeStatus,
} from "../../contracts/domains";
import type { PublicationStatus } from "../../api/fileTypes";

export interface FileOutcome {
  name: string;
  target: string;
  status: PublicationStatus;
  message?: string;
}

// Shared counts for notices and result details. Operation-level cancellation or
// uncertainty remains separate: encountered entries cannot describe unread work.
export function summarizeFileOutcomes(outcomes: readonly FileOutcome[]) {
  const counts = {
    completed: 0,
    skipped: 0,
    failed: 0,
    cancelled: 0,
    unconfirmed: 0,
  };
  for (const { status } of outcomes) {
    switch (status) {
      case BrowserPublicationStatus.Uploaded:
      case ExtractionOutcomeStatus.Extracted:
      case ExtractionOutcomeStatus.Merged:
      case ExtractionOutcomeStatus.Overwritten:
      case ExtractionOutcomeStatus.Renamed:
        counts.completed++;
        break;
      default:
        counts[status]++;
    }
  }
  return counts;
}
