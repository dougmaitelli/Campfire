import { expect, test, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
import type { ISearchResultChangeEvent } from "@xterm/addon-search";
import userEvent from "@testing-library/user-event";
import { ServerTerminal } from "../../src/components/servers/ServerTerminal";

const mocks = vi.hoisted(() => ({
  next: vi.fn(() => true),
  previous: vi.fn(() => true),
  focus: vi.fn(),
  clearSelection: vi.fn(),
  clearDecorations: vi.fn(),
  disposeResults: vi.fn(),
  results: undefined as
    ((results: ISearchResultChangeEvent) => void) | undefined,
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
    clearDecorations = mocks.clearDecorations;
    onDidChangeResults(listener: (results: ISearchResultChangeEvent) => void) {
      mocks.results = listener;
      return { dispose: mocks.disposeResults };
    }
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
  const view = render(
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
  expect(mocks.next).toHaveBeenLastCalledWith(
    "error",
    expect.objectContaining({
      incremental: true,
      decorations: expect.any(Object),
    }),
  );
  act(() => mocks.results?.({ resultIndex: 0, resultCount: 3 }));
  expect(screen.getByRole("status", { name: "Match 1 of 3" }).textContent).toBe(
    "1 / 3",
  );
  await user.keyboard("{Enter}");
  expect(mocks.next).toHaveBeenLastCalledWith(
    "error",
    expect.objectContaining({ incremental: false }),
  );
  act(() => mocks.results?.({ resultIndex: 1, resultCount: 3 }));
  expect(screen.getByRole("status", { name: "Match 2 of 3" }).textContent).toBe(
    "2 / 3",
  );
  await user.keyboard("{Shift>}{Enter}{/Shift}");
  expect(mocks.previous).toHaveBeenLastCalledWith(
    "error",
    expect.objectContaining({ incremental: false }),
  );
  act(() => mocks.results?.({ resultIndex: 0, resultCount: 4 }));
  expect(screen.getByRole("status", { name: "Match 1 of 4" }).textContent).toBe(
    "1 / 4",
  );
  act(() => mocks.results?.({ resultIndex: -1, resultCount: 1000 }));
  expect(screen.getByText("– / 1,000+")).toBeDefined();
  mocks.next.mockReturnValueOnce(false);
  await user.click(screen.getByRole("button", { name: "Next" }));
  expect(screen.getByText("No matches")).toBeDefined();
  await user.clear(input);
  expect(screen.queryByText("No matches")).toBeNull();
  expect(mocks.clearDecorations).toHaveBeenCalled();
  expect(screen.queryByText("– / 1,000+")).toBeNull();
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("search")).toBeNull();
  expect(mocks.focus).toHaveBeenCalled();
  fireEvent.keyDown(command, { key: "f", metaKey: true });
  expect(screen.getByRole("search")).toBeDefined();
  view.unmount();
  expect(mocks.disposeResults).toHaveBeenCalled();
  vi.unstubAllGlobals();
});
