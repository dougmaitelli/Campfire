import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { CrashDetails } from "../../src/components/activity/CrashDetails";
import { isCrashRecord, type CrashRecord } from "../../src/api/crashes";
import { CrashReason } from "../../src/contracts/domains";

const record: CrashRecord = {
  id: "a".repeat(64),
  container_id: "b".repeat(64),
  exited_at: 1792670400,
  exit_code: 137,
  reason: CrashReason.Unknown,
  console: "private console evidence",
  console_unavailable: false,
  console_truncated: true,
};
afterEach(cleanup);

test.each([true, false])(
  "crash evidence respects console access: %s",
  (canReadConsole) => {
    render(<CrashDetails record={record} canReadConsole={canReadConsole} />);
    expect(screen.getByText("137")).toBeDefined();
    expect(screen.queryByText(record.console!) !== null).toBe(canReadConsole);
    expect(screen.queryByText("Recent console output") !== null).toBe(
      canReadConsole,
    );
    expect(screen.getByText("Reason")).toBeDefined();
    expect(screen.getByText("Unknown")).toBeDefined();
    expect(screen.queryByText("Out of memory")).toBeNull();
    expect(screen.queryByText("Started")).toBeNull();
    expect(screen.queryByText("Recorded")).toBeNull();
  },
);

test.each([CrashReason.Unknown, CrashReason.OutOfMemory])(
  "crash reason: %s",
  (reason) => {
    render(<CrashDetails record={{ ...record, reason }} canReadConsole />);
    expect(
      screen.getByText(
        reason === CrashReason.Unknown ? "Unknown" : "Out of memory",
      ),
    ).toBeDefined();
  },
);

test("unavailable console evidence does not discard exit metadata", () => {
  render(
    <CrashDetails
      record={{ ...record, console_unavailable: true }}
      canReadConsole
    />,
  );
  expect(screen.getByText("137")).toBeDefined();
  expect(screen.queryByText(record.console!)).toBeNull();
  expect(
    screen.getByText(/Console evidence could not be captured/),
  ).toBeDefined();
});

test("crash contracts validate identity, bounded UTF-8 evidence, exit time and reason", () => {
  expect(isCrashRecord(record)).toBe(true);
  for (const invalid of [
    { ...record, id: "wrong" },
    { ...record, container_id: "wrong" },
    { ...record, exit_code: 256 },
    { ...record, exit_code: 1.5 },
    { ...record, exited_at: 0 },
    { ...record, reason: "guessed" },
    { ...record, reason: undefined },
    { ...record, reason: null },
    { ...record, console: "é".repeat(8193) },
    { ...record, console_truncated: undefined },
  ])
    expect(isCrashRecord(invalid)).toBe(false);
});
