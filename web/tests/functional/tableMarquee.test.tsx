import { useState } from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { Table, TableRow } from "../../src/components/table/Table";
import { MarqueeSelection } from "../../src/components/table/MarqueeSelection";
import { useTableSelection } from "../../src/components/table/useTableSelection";

class TestPointerEvent extends MouseEvent {
  pointerId: number;
  pointerType: string;
  constructor(type: string, init: PointerEventInit = {}) {
    super(type, init);
    this.pointerId = init.pointerId ?? 1;
    this.pointerType = init.pointerType ?? "mouse";
  }
}
const frames: FrameRequestCallback[] = [];
beforeEach(() => {
  frames.length = 0;
  vi.stubGlobal("PointerEvent", TestPointerEvent);
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
    frames.push(callback);
    return frames.length;
  });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function Inventory({
  disabled = false,
  ids = ["Alpha", "Beta", "Charlie"],
}: {
  disabled?: boolean;
  ids?: string[];
}) {
  const [selected, setSelected] = useState<string[]>(["Alpha"]);
  const selection = useTableSelection({
    ids,
    disabled,
    scope: "inventory",
    selectedIDs: selected,
    onSelectionChange: setSelected,
  });
  return (
    <>
      <MarqueeSelection selection={selection}>
        <Table>
          <tbody>
            {ids.map((id) => (
              <TableRow key={id} {...selection.rowProps(id)}>
                <td>{id}</td>
                <td>
                  <button>Action {id}</button>
                </td>
              </TableRow>
            ))}
          </tbody>
        </Table>
      </MarqueeSelection>
      <output aria-label="Selection">{selected.join(",")}</output>
    </>
  );
}
function geometry() {
  const root = document.querySelector<HTMLDivElement>(
    ".table-selection-surface",
  )!;
  Object.defineProperties(root, {
    scrollWidth: { configurable: true, value: 300 },
    scrollHeight: { configurable: true, value: 400 },
    setPointerCapture: { configurable: true, value: vi.fn() },
    releasePointerCapture: { configurable: true, value: vi.fn() },
    hasPointerCapture: { configurable: true, value: () => true },
  });
  vi.spyOn(root, "getBoundingClientRect").mockReturnValue(
    new DOMRect(0, 0, 300, 400),
  );
  for (const [index, row] of [...root.querySelectorAll("tr")].entries())
    vi.spyOn(row, "getBoundingClientRect").mockReturnValue(
      new DOMRect(0, 50 + index * 40, 300, 40),
    );
  return root;
}
function begin(
  target: Element = screen.getByText("Beta"),
  extra: PointerEventInit = {},
) {
  fireEvent.pointerDown(target, {
    pointerId: 1,
    pointerType: "mouse",
    button: 0,
    clientX: 200,
    clientY: 95,
    ...extra,
  });
}
function move() {
  fireEvent.pointerMove(window, { pointerId: 1, clientX: 220, clientY: 165 });
}

test("marquee selects intersecting generic rows and consumes the trailing click", () => {
  render(<Inventory />);
  geometry();
  begin();
  move();
  expect(screen.getByLabelText("Selection").textContent).toBe("Beta,Charlie");
  expect(document.querySelector(".table-selection-marquee")).not.toBeNull();
  fireEvent.pointerUp(window, { pointerId: 1 });
  fireEvent.click(screen.getByText("Charlie"));
  expect(screen.getByLabelText("Selection").textContent).toBe("Beta,Charlie");
  expect(document.querySelector(".table-selection-marquee")).toBeNull();
});

test("marquee works inside an enclosing disclosure and still excludes its own controls", () => {
  render(
    <details open>
      <summary>Inventory</summary>
      <Inventory />
    </details>,
  );
  geometry();
  begin(screen.getByRole("button", { name: "Action Beta" }));
  move();
  expect(screen.getByLabelText("Selection").textContent).toBe("Alpha");
  begin();
  move();
  expect(screen.getByLabelText("Selection").textContent).toBe("Beta,Charlie");
  fireEvent.pointerUp(window, { pointerId: 1 });
});

test("additive marquee retains the baseline and Escape restores it", () => {
  render(<Inventory />);
  geometry();
  begin(screen.getByText("Beta"), { ctrlKey: true });
  move();
  expect(screen.getByLabelText("Selection").textContent).toBe(
    "Alpha,Beta,Charlie",
  );
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.getByLabelText("Selection").textContent).toBe("Alpha");
  expect(document.querySelector(".table-selection-marquee")).toBeNull();
});

test.each(["touch", "control", "draggable", "disabled"])(
  "%s keeps its native behavior instead of starting a marquee",
  (kind) => {
    render(<Inventory disabled={kind === "disabled"} />);
    geometry();
    const target =
      kind === "control"
        ? screen.getByRole("button", { name: "Action Beta" })
        : screen.getByText("Beta");
    if (kind === "draggable") target.setAttribute("draggable", "true");
    begin(target, { pointerType: kind === "touch" ? "touch" : "mouse" });
    move();
    expect(screen.getByLabelText("Selection").textContent).toBe("Alpha");
    expect(document.querySelector(".table-selection-marquee")).toBeNull();
  },
);

test("changed rows terminate an active gesture and prune missing selections", () => {
  const view = render(<Inventory />);
  geometry();
  begin();
  move();
  view.rerender(<Inventory ids={["Alpha", "Charlie"]} />);
  expect(document.querySelector(".table-selection-marquee")).toBeNull();
  expect(screen.getByLabelText("Selection").textContent).toBe("Charlie");
  fireEvent.pointerMove(window, { pointerId: 1, clientX: 200, clientY: 50 });
  expect(screen.getByLabelText("Selection").textContent).toBe("Charlie");
});

test("edge scrolling advances while the pointer is held and cleanup cancels its animation", () => {
  const view = render(<Inventory />);
  geometry();
  begin();
  const initial = document.documentElement.scrollTop;
  fireEvent.pointerMove(window, {
    pointerId: 1,
    clientX: 220,
    clientY: window.innerHeight - 2,
  });
  act(() => frames[0]?.(0));
  expect(document.documentElement.scrollTop).toBeGreaterThan(initial);
  view.unmount();
  expect(cancelAnimationFrame).toHaveBeenCalled();
  document.documentElement.scrollTop = initial;
});
