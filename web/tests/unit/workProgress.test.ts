import { expect, test } from "vitest";
import { isWorkProgress } from "../../src/api/workProgress";

const valid = {
  phase: "extracting",
  input_bytes: 32,
  input_total: 64,
  output_bytes: 256,
  entries: 2,
  updated_at: "2026-11-18T12:00:00Z",
};

test("progress accepts unknown totals and independent input/output counts", () => {
  expect(isWorkProgress(valid)).toBe(true);
  expect(isWorkProgress({ ...valid, input_total: 0 })).toBe(true);
  const unknownTotal: Record<string, unknown> = { ...valid };
  delete unknownTotal.input_total;
  expect(isWorkProgress(unknownTotal)).toBe(true);
});

test.each([
  { input_bytes: -1 },
  { output_bytes: Infinity },
  { entries: 1.5 },
  { input_total: -1 },
  { input_total: 16 },
  { input_total: "64" },
  { input_bytes: Number.MAX_SAFE_INTEGER + 1 },
  { phase: "succeeded" },
  { updated_at: "invalid" },
  { entries: null },
])("rejects malformed progress %j", (change) => {
  expect(isWorkProgress({ ...valid, ...change })).toBe(false);
});
