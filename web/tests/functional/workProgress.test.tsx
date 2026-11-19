import { act, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { WorkProgress } from "../../src/components/status/WorkProgress";
import { InlineMessage } from "../../src/components/status/InlineMessage";
import {
  WorkPhase,
  type WorkProgress as Measurement,
} from "../../src/api/workProgress";

afterEach(() => vi.useRealTimers());
const snapshot = (changes: Partial<Measurement> = {}): Measurement => ({
  phase: WorkPhase.Uploading,
  input_bytes: 32,
  input_total: 32,
  output_bytes: 0,
  entries: 0,
  updated_at: new Date().toISOString(),
  ...changes,
});

test("all bytes supplied still shows unconfirmed publication, not success", () => {
  render(<WorkProgress progress={snapshot()} />);
  expect(
    screen
      .getByRole("progressbar", { name: "Bytes supplied to storage" })
      .getAttribute("value"),
  ).toBe("32");
  expect(
    screen.getByText(
      "Measured total reached; the operation is still in progress.",
    ),
  ).toBeTruthy();
  expect(screen.queryByText(/complete|succeeded/i)).toBeNull();
});

test("unknown source totals stay indeterminate and separate compressed output", () => {
  render(
    <WorkProgress
      progress={snapshot({
        phase: WorkPhase.Archiving,
        input_total: 0,
        output_bytes: 4,
        entries: 1,
      })}
    />,
  );
  expect(
    screen
      .getByRole("progressbar", { name: "Source payload processed" })
      .getAttribute("value"),
  ).toBeNull();
  expect(screen.getByText(/Compressed output:.*1 archive entry/)).toBeTruthy();
  expect(screen.queryByText(/Measured total reached/)).toBeNull();
});

test("stale measurements stop presenting a live meter and recover on new snapshots", () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-11-18T14:00:00Z"));
  const view = render(<WorkProgress progress={snapshot()} />);
  act(() => vi.advanceTimersByTime(35000));
  expect(screen.queryByRole("progressbar")).toBeNull();
  expect(screen.getByText(/No recent measurements/)).toBeTruthy();
  view.rerender(<WorkProgress progress={snapshot({ input_bytes: 16 })} />);
  expect(screen.getByRole("progressbar")).toBeTruthy();
  expect(screen.queryByText(/No recent measurements/)).toBeNull();
});

test.each([-120000, 120000])(
  "staleness uses locally observed changes despite clock skew %i",
  (skew) => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-11-18T14:00:00Z"));
    let measurement = snapshot({
      input_bytes: 1,
      updated_at: new Date(Date.now() + skew).toISOString(),
    });
    const view = render(
      <WorkProgress progress={measurement} operationID="first" />,
    );
    expect(screen.getByRole("progressbar")).toBeTruthy();
    for (let index = 0; index < 7; index++) {
      act(() => vi.advanceTimersByTime(5000));
      // Polling the same measurement must not postpone stale detection.
      view.rerender(
        <WorkProgress progress={{ ...measurement }} operationID="first" />,
      );
    }
    expect(screen.queryByRole("progressbar")).toBeNull();
    measurement = {
      ...measurement,
      input_bytes: 16,
      updated_at: new Date(Date.now() + skew).toISOString(),
    };
    view.rerender(<WorkProgress progress={measurement} operationID="first" />);
    expect(screen.getByRole("progressbar")).toBeTruthy();
    act(() => vi.advanceTimersByTime(30000));
    expect(screen.queryByRole("progressbar")).toBeNull();
    view.rerender(<WorkProgress progress={measurement} operationID="second" />);
    expect(screen.getByRole("progressbar")).toBeTruthy();
  },
);

test("phase changes start a fresh deadline and timers are cleaned up", () => {
  vi.useFakeTimers();
  const measurement = snapshot();
  const view = render(<WorkProgress progress={measurement} />);
  act(() => vi.advanceTimersByTime(30000));
  expect(screen.getByText(/No recent measurements/)).toBeTruthy();
  view.rerender(
    <WorkProgress progress={{ ...measurement, phase: WorkPhase.Committing }} />,
  );
  expect(screen.queryByText(/No recent measurements/)).toBeNull();
  view.unmount();
  expect(vi.getTimerCount()).toBe(0);
});

test("progress is valid inside paragraph-based status messages", () => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  const view = render(
    <InlineMessage tone="info">
      <WorkProgress progress={snapshot()} />
    </InlineMessage>,
  );
  expect(view.container.querySelector("p > span.work-progress")).not.toBeNull();
  expect(view.container.querySelector("p div")).toBeNull();
  expect(errors).not.toHaveBeenCalled();
});

test("publication has no inherited byte percentage", () => {
  render(
    <WorkProgress
      progress={snapshot({
        phase: WorkPhase.Committing,
        input_bytes: 0,
        input_total: 0,
      })}
    />,
  );
  expect(
    screen.getByRole("group", { name: "Confirming publication" }),
  ).toBeTruthy();
  expect(screen.queryByRole("progressbar")).toBeNull();
});
