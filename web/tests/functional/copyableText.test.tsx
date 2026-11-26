import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { CopyableText } from "../../src/components/forms/CopyableText";

afterEach(cleanup);
const labels = {
  copyLabel: "Copy text",
  copiedLabel: "Copied text",
  failedLabel: "Select and copy manually.",
};
function clipboard(writeText: ReturnType<typeof vi.fn>) {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
}

test("copyable links retain their display and copy the full value with feedback", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  clipboard(writeText);
  render(
    <CopyableText value="https://example.com/status/community" {...labels}>
      <a href="/status/community">/status/community</a>
    </CopyableText>,
  );
  expect(screen.getByRole("link").getAttribute("href")).toBe(
    "/status/community",
  );
  fireEvent.click(screen.getByRole("button", { name: "Copy text" }));
  await screen.findByRole("button", { name: "Copied text" });
  expect(screen.getByRole("status").textContent).toBe("Copied text");
  expect(writeText).toHaveBeenCalledWith(
    "https://example.com/status/community",
  );
});

test("clipboard failures show manual-copy guidance and can be retried", async () => {
  const writeText = vi
    .fn()
    .mockRejectedValueOnce(new Error("Denied"))
    .mockResolvedValue(undefined);
  clipboard(writeText);
  render(<CopyableText value="secret" {...labels} />);
  fireEvent.click(screen.getByRole("button", { name: "Copy text" }));
  await screen.findByText(labels.failedLabel);
  expect(screen.getByText("secret")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Copy text" }));
  await screen.findByRole("button", { name: "Copied text" });
  expect(screen.queryByText(labels.failedLabel)).toBeNull();
});

test("missing clipboard support does not hide the selectable text", async () => {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: undefined,
  });
  render(<CopyableText value="secret" {...labels} />);
  fireEvent.click(screen.getByRole("button", { name: "Copy text" }));
  await screen.findByText(labels.failedLabel);
  expect(screen.getByText("secret")).toBeTruthy();
});

test("copy feedback belongs to the current value and disabled controls cannot copy", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  clipboard(writeText);
  const view = render(<CopyableText value="old" {...labels} />);
  fireEvent.click(screen.getByRole("button", { name: "Copy text" }));
  await screen.findByRole("button", { name: "Copied text" });
  view.rerender(<CopyableText value="new" disabled {...labels} />);
  fireEvent.click(screen.getByRole("button", { name: "Copy text" }));
  expect(screen.getByRole("status").textContent).toBe("");
  expect(writeText).toHaveBeenCalledTimes(1);
});
