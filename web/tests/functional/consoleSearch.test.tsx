import { expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ServerTerminal } from "../../src/components/servers/ServerTerminal";

const mocks = vi.hoisted(() => ({
  next: vi.fn(() => true),
  previous: vi.fn(() => true),
  focus: vi.fn(),
  clearSelection: vi.fn(),
}));

vi.mock("@xterm/xterm", () => ({
  Terminal: class {
    loadAddon() {}
    open() {}
    dispose() {}
    focus = mocks.focus;
    clearSelection = mocks.clearSelection;
  },
}));
vi.mock("@xterm/addon-fit", () => ({
  FitAddon: class {
    fit() {}
  },
}));
vi.mock("@xterm/addon-search", () => ({
  SearchAddon: class {
    findNext = mocks.next;
    findPrevious = mocks.previous;
  },
}));
vi.mock("../../src/components/servers/consoleStream", () => ({
  connectConsole: vi.fn(),
}));

test("console search supports shortcuts, navigation, empty results, and closing", async () => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  const user = userEvent.setup();
  render(
    <ServerTerminal
      serverID="server"
      streamKey="stream"
      streamMode="live"
      onBusyChange={() => {}}
    />,
  );
  const command = screen.getByRole("textbox", { name: "Command" });
  const shortcut = new KeyboardEvent("keydown", {
    key: "f",
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  fireEvent(command, shortcut);
  expect(shortcut.defaultPrevented).toBe(true);
  const input = screen.getByRole("textbox", { name: "Search console" });
  expect(document.activeElement).toBe(input);
  await user.type(input, "error");
  expect(mocks.next).toHaveBeenLastCalledWith("error", { incremental: true });
  await user.keyboard("{Enter}");
  expect(mocks.next).toHaveBeenLastCalledWith("error", { incremental: false });
  await user.keyboard("{Shift>}{Enter}{/Shift}");
  expect(mocks.previous).toHaveBeenLastCalledWith("error", {
    incremental: false,
  });
  mocks.next.mockReturnValueOnce(false);
  await user.click(screen.getByRole("button", { name: "Next match" }));
  expect(screen.getByText("No matches")).toBeDefined();
  await user.clear(input);
  expect(screen.queryByText("No matches")).toBeNull();
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("search")).toBeNull();
  expect(mocks.focus).toHaveBeenCalled();
  fireEvent.keyDown(command, { key: "f", metaKey: true });
  expect(screen.getByRole("search")).toBeDefined();
  vi.unstubAllGlobals();
});
