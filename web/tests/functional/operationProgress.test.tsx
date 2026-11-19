import { render, screen } from "@testing-library/react";
import { describe, expect, test } from "vitest";
import { OperationProgress } from "../../src/components/status/OperationProgress";

describe("shared operation measurements", () => {
  test("preserves real percentages without declaring completion at 100%", () => {
    const view = render(
      <OperationProgress label="Archive bytes copied" value={25} total={100} />,
    );
    expect(screen.getByText("25%")).toBeTruthy();
    expect(screen.getByRole("progressbar").getAttribute("value")).toBe("25");
    view.rerender(
      <OperationProgress
        label="Archive bytes copied"
        value={100}
        total={100}
      />,
    );
    expect(screen.getByText("100%")).toBeTruthy();
    expect(screen.getByText(/Measured total reached/)).toBeTruthy();
    view.rerender(
      <OperationProgress
        label="Archive bytes copied"
        value={100}
        total={100}
        active={false}
      />,
    );
    expect(screen.getByText("100%")).toBeTruthy();
    expect(screen.queryByRole("progressbar")).toBeNull();
    expect(screen.queryByText(/Measured total reached/)).toBeNull();
  });
  test("unknown totals remain indeterminate and stale data has no live meter", () => {
    const view = render(<OperationProgress label="Payload" value={500} />);
    expect(screen.getByRole("progressbar").hasAttribute("value")).toBe(false);
    expect(screen.queryByText(/%/)).toBeNull();
    view.rerender(
      <OperationProgress label="Payload" value={500} total={1000} stale />,
    );
    expect(screen.queryByRole("progressbar")).toBeNull();
    expect(screen.getByText("50%")).toBeTruthy();
    expect(screen.getByText(/No recent measurements/)).toBeTruthy();
  });
});
