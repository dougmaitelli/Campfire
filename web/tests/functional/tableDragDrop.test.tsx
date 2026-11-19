import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { useTableDragDrop } from "../../src/components/table/useTableDragDrop";
import { useExternalFileDrop } from "../../src/components/dragDrop/useExternalFileDrop";
import type { ExternalFileDrop } from "../../src/components/dragDrop/useExternalFileDrop";

function transfer(extra: string[] = []) {
  const values = new Map<string, string>();
  return {
    get types() {
      return [...values.keys(), ...extra];
    },
    clearData: () => values.clear(),
    setData: (key: string, value: string) => values.set(key, value),
    getData: (key: string) => values.get(key) ?? "",
    effectAllowed: "none",
    dropEffect: "none",
    files: [],
    items: [],
  };
}
function Board({
  scope = "board",
  disabled = false,
  onDrop,
  onDrag = () => {},
}: {
  scope?: string;
  disabled?: boolean;
  onDrop: (ids: readonly string[], target: string) => void;
  onDrag?: (ids: string[]) => void;
}) {
  const drag = useTableDragDrop({
    scope,
    disabled,
    getItems: (id) => (id === "selected" ? ["one", "two"] : [id]),
    canDrop: (_, target) => target !== "blocked",
    onDrop,
    onDragItems: onDrag,
  });
  return (
    <>
      <button {...drag.sourceProps("selected")}>Selected source</button>
      <button {...drag.sourceProps("single")}>Single source</button>
      <button {...drag.targetProps("outside")}>Outside target</button>
      <div {...drag.targetProps("blocked")}>Blocked target</div>
      <output aria-label="Dragging">{drag.draggedIDs.join(",")}</output>
    </>
  );
}

function RowSources({
  onDrop,
}: {
  onDrop: (ids: readonly string[], target: string) => void;
}) {
  const drag = useTableDragDrop({
    scope: "rows",
    getItems: (id) => [id],
    canDrop: () => true,
    onDrop,
  });
  return (
    <details open>
      <summary>Records</summary>
      <div {...drag.sourceProps("row")} aria-label="Row source">
        <span>Row space</span>
        <button>Action</button>
        <input aria-label="Checkbox" type="checkbox" />
        <input aria-label="Text" />
        <button {...drag.sourceProps("handle")}>Handle</button>
        <button {...drag.sourceProps("disabled", false)}>
          Disabled handle
        </button>
      </div>
      <div {...drag.targetProps("destination")}>Destination</div>
    </details>
  );
}

test.each(["Action", "Checkbox", "Text", "Disabled handle"])(
  "protects %s when native dragstart targets its draggable ancestor",
  (name) => {
    const drop = vi.fn();
    render(<RowSources onDrop={drop} />);
    const control =
      name === "Action" || name === "Disabled handle"
        ? screen.getByRole("button", { name })
        : screen.getByLabelText(name);
    fireEvent.pointerDown(control);
    const dataTransfer = transfer();
    expect(
      fireEvent.dragStart(screen.getByLabelText("Row source"), {
        dataTransfer,
      }),
    ).toBe(false);
    fireEvent.drop(screen.getByText("Destination"), { dataTransfer });
    expect(drop).not.toHaveBeenCalled();
    // A subsequent gesture on row space must not inherit the rejected origin.
    fireEvent.pointerDown(screen.getByText("Row space"));
    fireEvent.dragStart(screen.getByLabelText("Row source"), { dataTransfer });
    fireEvent.drop(screen.getByText("Destination"), { dataTransfer });
    expect(drop).toHaveBeenCalledExactlyOnceWith(["row"], "destination");
  },
);

test("an explicit button handle inside a draggable row owns its gesture", () => {
  const drop = vi.fn();
  render(<RowSources onDrop={drop} />);
  const handle = screen.getByRole("button", { name: "Handle" });
  const dataTransfer = transfer();
  fireEvent.pointerDown(handle);
  fireEvent.dragStart(handle, { dataTransfer });
  fireEvent.drop(screen.getByText("Destination"), { dataTransfer });
  expect(drop).toHaveBeenCalledExactlyOnceWith(["handle"], "destination");
});

test("internal drag extension works without selection or a table, with group and single-item intents", () => {
  const drop = vi.fn();
  const start = vi.fn();
  render(<Board onDrop={drop} onDrag={start} />);
  const dataTransfer = transfer();
  fireEvent.dragStart(screen.getByText("Selected source"), { dataTransfer });
  expect(start).toHaveBeenCalledWith(["one", "two"]);
  expect(dataTransfer.effectAllowed).toBe("move");
  expect(screen.getByLabelText("Dragging").textContent).toBe("one,two");
  fireEvent.dragOver(screen.getByText("Outside target"), { dataTransfer });
  expect(dataTransfer.dropEffect).toBe("move");
  expect(
    screen.getByText("Outside target").getAttribute("data-drop-target"),
  ).toBe("true");
  fireEvent.drop(screen.getByText("Outside target"), { dataTransfer });
  expect(drop).toHaveBeenCalledExactlyOnceWith(["one", "two"], "outside");
  expect(screen.getByLabelText("Dragging").textContent).toBe("");
  fireEvent.dragStart(screen.getByText("Single source"), { dataTransfer });
  fireEvent.drop(screen.getByText("Outside target"), { dataTransfer });
  expect(drop).toHaveBeenLastCalledWith(["single"], "outside");
});

test("invalid targets, OS files, forged payloads, scope changes and disabling never emit a move", () => {
  const drop = vi.fn();
  const view = render(<Board onDrop={drop} />);
  let dataTransfer = transfer();
  const source = () => screen.getByText("Single source");
  const target = () => screen.getByText("Outside target");
  fireEvent.dragStart(source(), { dataTransfer });
  fireEvent.dragOver(screen.getByText("Blocked target"), { dataTransfer });
  expect(dataTransfer.dropEffect).toBe("none");
  fireEvent.drop(screen.getByText("Blocked target"), { dataTransfer });
  fireEvent.dragStart(source(), { dataTransfer });
  const key = dataTransfer.types[0]!;
  dataTransfer.setData(key, "not the active token");
  fireEvent.drop(target(), { dataTransfer });
  dataTransfer = transfer(["Files"]);
  fireEvent.dragStart(source(), { dataTransfer });
  fireEvent.drop(target(), { dataTransfer });
  dataTransfer = transfer();
  fireEvent.dragStart(source(), { dataTransfer });
  view.rerender(<Board onDrop={drop} scope="another board" />);
  fireEvent.drop(target(), { dataTransfer });
  fireEvent.dragStart(source(), { dataTransfer });
  view.rerender(<Board onDrop={drop} scope="another board" disabled />);
  fireEvent.drop(target(), { dataTransfer });
  expect(drop).not.toHaveBeenCalled();
  expect(screen.getByLabelText("Dragging").textContent).toBe("");
});

test("cancelled native starts and another controller's targets never create a move", () => {
  const drop = vi.fn();
  const view = render(
    <div onDragStartCapture={(event) => event.preventDefault()}>
      <Board onDrop={drop} />
    </div>,
  );
  const dataTransfer = transfer();
  fireEvent.dragStart(screen.getByText("Single source"), { dataTransfer });
  expect(screen.getByLabelText("Dragging").textContent).toBe("");
  fireEvent.drop(screen.getByText("Outside target"), { dataTransfer });
  expect(drop).not.toHaveBeenCalled();
  view.unmount();
  render(
    <>
      <Board onDrop={drop} />
      <Board onDrop={drop} />
    </>,
  );
  fireEvent.dragStart(screen.getAllByText("Single source")[0]!, {
    dataTransfer,
  });
  fireEvent.drop(screen.getAllByText("Outside target")[1]!, { dataTransfer });
  expect(drop).not.toHaveBeenCalled();
});

test.each(["Escape", "blur", "dragend"])(
  "%s cancels the drag and rejects stale drops",
  (action) => {
    const drop = vi.fn();
    render(<Board onDrop={drop} />);
    const dataTransfer = transfer();
    fireEvent.dragStart(screen.getByText("Single source"), { dataTransfer });
    if (action === "Escape") fireEvent.keyDown(window, { key: "Escape" });
    else fireEvent(window, new Event(action));
    fireEvent.drop(screen.getByText("Outside target"), { dataTransfer });
    expect(drop).not.toHaveBeenCalled();
  },
);

function UploadSurface({
  enabled = true,
  onDrop,
}: {
  enabled?: boolean;
  onDrop: (drop: ExternalFileDrop) => void;
}) {
  const drop = useExternalFileDrop({ enabled, onDrop });
  return (
    <section {...drop.dropProps} aria-label="Generic attachment surface">
      {drop.dragging ? "Release attachments" : "Idle"}
      <span>Child</span>
    </section>
  );
}

test("external drop attaches to any element, ignores internal drags and emits files without starting uploads", () => {
  const drop = vi.fn();
  render(<UploadSurface onDrop={drop} />);
  const target = screen.getByLabelText("Generic attachment surface");
  const internal = transfer(["application/x-campfire-table-items"]);
  fireEvent.dragEnter(target, { dataTransfer: internal });
  fireEvent.drop(target, { dataTransfer: internal });
  expect(target.textContent).toContain("Idle");
  expect(drop).not.toHaveBeenCalled();
  const file = new File(["hello"], "note.txt");
  const dataTransfer = { ...transfer(["Files"]), files: [file] };
  fireEvent.dragEnter(target, { dataTransfer });
  fireEvent.dragEnter(screen.getByText("Child"), { dataTransfer });
  fireEvent.dragLeave(screen.getByText("Child"), { dataTransfer });
  expect(target.textContent).toContain("Release attachments");
  fireEvent.dragOver(target, { dataTransfer });
  expect(dataTransfer.dropEffect).toBe("copy");
  fireEvent.drop(target, { dataTransfer });
  expect(drop).toHaveBeenCalledExactlyOnceWith({
    files: [file],
    hasDirectories: false,
  });
  expect(target.textContent).toContain("Idle");
});

test("external drop reports directories to consumer policy, resets when disabled, and guards browser navigation", () => {
  const drop = vi.fn();
  const view = render(<UploadSurface onDrop={drop} />);
  const target = screen.getByLabelText("Generic attachment surface");
  const dataTransfer = {
    ...transfer(["Files"]),
    items: [{ webkitGetAsEntry: () => ({ isDirectory: true }) }],
  };
  fireEvent.drop(target, { dataTransfer });
  expect(drop).toHaveBeenCalledExactlyOnceWith({
    files: [],
    hasDirectories: true,
  });
  fireEvent.dragEnter(target, { dataTransfer });
  view.rerender(<UploadSurface onDrop={drop} enabled={false} />);
  expect(target.textContent).toContain("Idle");
  fireEvent.drop(target, { dataTransfer });
  expect(drop).toHaveBeenCalledTimes(1);
  expect(fireEvent.dragOver(document.body, { dataTransfer })).toBe(false);
  expect(dataTransfer.dropEffect).toBe("none");
});
