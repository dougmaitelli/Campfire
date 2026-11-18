import { expect, test } from "vitest";
import type { PublicationStatus } from "../../src/api/fileTypes";
import { summarizeFileOutcomes } from "../../src/components/files/fileOutcomes";

test("empty results do not infer operation completion or cancellation", () => {
  expect(summarizeFileOutcomes([])).toEqual({
    completed: 0,
    skipped: 0,
    failed: 0,
    cancelled: 0,
    unconfirmed: 0,
  });
});

test("summaries count all publication outcomes without changing the results", () => {
  const statuses: PublicationStatus[] = [
    "uploaded",
    "extracted",
    "merged",
    "overwritten",
    "renamed",
    "skipped",
    "failed",
    "cancelled",
    "unconfirmed",
    "unconfirmed",
  ];
  const outcomes = statuses.map((status, index) =>
    Object.freeze({ name: `file-${index}`, target: `file-${index}`, status }),
  );
  expect(summarizeFileOutcomes(Object.freeze(outcomes))).toEqual({
    completed: 5,
    skipped: 1,
    failed: 1,
    cancelled: 1,
    unconfirmed: 2,
  });
  expect(outcomes.map((item) => item.status)).toEqual(statuses);
});
