import { useState } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { Table, TableRow } from "../../src/components/table/Table";
import { TableSelectionHelp } from "../../src/components/table/TableSelectionHelp";
import { TableSelectionCheckbox } from "../../src/components/table/TableSelectionCheckbox";
import { useTableSelection } from "../../src/components/table/useTableSelection";

const defaults = ["Alpha", "Beta", "Bravo", "Charlie"];
function Inventory({
  ids = defaults,
  scope = "inventory",
  disabled = false,
  onOpen = () => {},
}: {
  ids?: string[];
  scope?: string;
  disabled?: boolean;
  onOpen?: (id: string) => void;
}) {
  const [selected, setSelected] = useState<string[]>([]);
  const selection = useTableSelection({
    ids,
    scope,
    disabled,
    selectedIDs: selected,
    onSelectionChange: setSelected,
    onActivate: onOpen,
  });
  return (
    <>
      <Table>
        <thead>
          <tr>
            <th>
              <TableSelectionCheckbox selection={selection} />
            </th>
            <th>Record</th>
          </tr>
        </thead>
        <tbody>
          {ids.map((id) => (
            <TableRow
              key={id}
              {...selection.rowProps(id)}
              onOpen={() => onOpen(id)}
            >
              <td>
                <TableSelectionCheckbox selection={selection} id={id} />
              </td>
              <td>
                <button {...selection.activationProps(id)}>{id}</button>
                <input aria-label={`Notes for ${id}`} />
              </td>
            </TableRow>
          ))}
          <TableRow>
            <td>
              <TableSelectionCheckbox selection={selection} id="unavailable" />
            </td>
            <td>Unavailable</td>
          </TableRow>
        </tbody>
      </Table>
      <button onClick={selection.clear}>Clear selection</button>
      <output aria-label="Selected records">{selected.join(",")}</output>
    </>
  );
}
function checkbox(id: string) {
  return screen.getByRole<HTMLInputElement>("checkbox", {
    name: `Select ${id}`,
  });
}
function row(id: string) {
  return screen.getByRole("button", { name: id }).closest("tr")!;
}
function selected() {
  return screen.getByLabelText("Selected records").textContent;
}

test("selection inside an enclosing disclosure ignores only controls within the row", () => {
  render(
    <details open>
      <summary>Inventory</summary>
      <Inventory />
    </details>,
  );
  fireEvent.click(row("Beta"));
  expect(selected()).toBe("Beta");
  fireEvent.click(screen.getByLabelText("Notes for Charlie"));
  expect(selected()).toBe("Beta");
  fireEvent.click(row("Charlie"), { shiftKey: true });
  expect(selected()).toBe("Beta,Bravo,Charlie");
});

test("selection help describes marquee only when explicitly enabled", () => {
  const view = render(<TableSelectionHelp />);
  expect(screen.queryByText(/Drag across unselected row space/)).toBeNull();
  view.rerender(<TableSelectionHelp marquee />);
  expect(screen.getByText(/Drag across unselected row space/)).not.toBeNull();
});

test("base row opening works inside an enclosing disclosure", () => {
  const open = vi.fn();
  render(
    <details open>
      <summary>Inventory</summary>
      <Table>
        <tbody>
          <TableRow onOpen={open}>
            <td>Record</td>
          </TableRow>
        </tbody>
      </Table>
    </details>,
  );
  fireEvent.click(screen.getByText("Record"));
  expect(open).toHaveBeenCalledOnce();
});

test("checkboxes, indeterminate select-all, disabled rows and clear work without file-browser code", async () => {
  const user = userEvent.setup();
  render(<Inventory />);
  const all = screen.getByRole<HTMLInputElement>("checkbox", {
    name: "Select all items",
  });
  expect(checkbox("unavailable").disabled).toBe(true);
  await user.click(checkbox("Beta"));
  expect(selected()).toBe("Beta");
  expect(all.indeterminate).toBe(true);
  await user.click(all);
  expect(selected()).toBe(defaults.join(","));
  expect(all.checked).toBe(true);
  await user.click(all);
  expect(selected()).toBe("");
  fireEvent.click(checkbox("Charlie"), { shiftKey: true });
  expect(selected()).toBe("Charlie");
  await user.click(screen.getByRole("button", { name: "Clear selection" }));
  await user.click(checkbox("Alpha"));
  await user.click(screen.getByRole("button", { name: "Clear selection" }));
  expect(selected()).toBe("");
});

test("range checkboxes use a stable anchor and can remove a range", () => {
  render(<Inventory />);
  fireEvent.click(checkbox("Beta"));
  fireEvent.click(checkbox("Charlie"), { shiftKey: true });
  expect(selected()).toBe("Beta,Bravo,Charlie");
  fireEvent.click(checkbox("Bravo"), { shiftKey: true });
  expect(selected()).toBe("Charlie");
  expect(checkbox("Beta").checked).toBe(false);
  expect(checkbox("Bravo").checked).toBe(false);
});

test("row space selects, modifiers extend, and activation buttons keep normal opening", () => {
  const open = vi.fn();
  render(<Inventory onOpen={open} />);
  fireEvent.click(row("Beta"));
  expect(selected()).toBe("Beta");
  expect(open).not.toHaveBeenCalled();
  fireEvent.click(row("Charlie"), { shiftKey: true });
  expect(selected()).toBe("Beta,Bravo,Charlie");
  fireEvent.click(row("Alpha"), { metaKey: true });
  expect(selected()).toBe("Beta,Bravo,Charlie,Alpha");
  fireEvent.click(row("Beta"), { ctrlKey: true });
  expect(selected()).toBe("Bravo,Charlie,Alpha");
  fireEvent.click(screen.getByRole("button", { name: "Beta" }), {
    ctrlKey: true,
  });
  expect(selected()).toBe("Bravo,Charlie,Alpha,Beta");
  expect(open).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Beta" }));
  expect(open).toHaveBeenCalledExactlyOnceWith("Beta");
  fireEvent.click(screen.getByLabelText("Notes for Charlie"));
  expect(selected()).toBe("Bravo,Charlie,Alpha,Beta");
});

test("keyboard navigation, ranges, select-all, clear, activation and type-to-select are scoped to rows", () => {
  const open = vi.fn();
  render(<Inventory onOpen={open} />);
  fireEvent.keyDown(row("Alpha"), { key: "ArrowDown", shiftKey: true });
  expect(selected()).toBe("Alpha,Beta");
  expect(document.activeElement).toBe(row("Beta"));
  fireEvent.keyDown(row("Beta"), { key: "End", ctrlKey: true });
  expect(document.activeElement).toBe(row("Charlie"));
  expect(selected()).toBe("Alpha,Beta");
  fireEvent.keyDown(row("Charlie"), { key: " " });
  expect(selected()).toBe("Alpha,Beta,Charlie");
  fireEvent.keyDown(row("Charlie"), { key: "a", metaKey: true });
  expect(selected()).toBe(defaults.join(","));
  fireEvent.keyDown(row("Charlie"), { key: "Escape" });
  expect(selected()).toBe("");
  fireEvent.keyDown(row("Charlie"), { key: "b" });
  expect(selected()).toBe("Beta");
  fireEvent.keyDown(row("Beta"), { key: "b" });
  expect(selected()).toBe("Bravo");
  fireEvent.keyDown(row("Bravo"), { key: "Enter" });
  expect(open).toHaveBeenCalledExactlyOnceWith("Bravo");
  fireEvent.keyDown(screen.getByLabelText("Notes for Alpha"), {
    key: "a",
    ctrlKey: true,
  });
  expect(selected()).toBe("Bravo");
});

test("typing an uppercase prefix selects a match rather than extending a range", () => {
  render(<Inventory />);
  fireEvent.click(row("Alpha"));
  fireEvent.keyDown(row("Alpha"), { key: "C", shiftKey: true });
  expect(selected()).toBe("Charlie");
});

test("clear resets the typeahead prefix", () => {
  render(<Inventory />);
  fireEvent.keyDown(row("Alpha"), { key: "c" });
  fireEvent.keyDown(row("Charlie"), { key: "h" });
  expect(selected()).toBe("Charlie");
  fireEvent.keyDown(row("Charlie"), { key: "Escape" });
  fireEvent.keyDown(row("Charlie"), { key: "a" });
  expect(selected()).toBe("Alpha");
});

test("visible ordering drives ranges, missing IDs are pruned, and scope changes clear selection", () => {
  const view = render(<Inventory />);
  fireEvent.click(checkbox("Beta"));
  view.rerender(<Inventory ids={["Charlie", "Beta", "Alpha"]} />);
  fireEvent.click(checkbox("Alpha"), { shiftKey: true });
  expect(selected()).toBe("Beta,Alpha");
  view.rerender(<Inventory ids={["Alpha", "Charlie"]} />);
  expect(selected()).toBe("Alpha");
  view.rerender(<Inventory ids={["Alpha", "Charlie"]} scope="other page" />);
  expect(selected()).toBe("");
  fireEvent.click(checkbox("Charlie"), { shiftKey: true });
  expect(selected()).toBe("Charlie");
});

test("disabled selection ignores mouse and keys", () => {
  render(<Inventory disabled />);
  fireEvent.click(row("Alpha"));
  fireEvent.keyDown(row("Alpha"), { key: "a", ctrlKey: true });
  expect(selected()).toBe("");
  expect(checkbox("Alpha").disabled).toBe(true);
  expect(row("Alpha").getAttribute("tabindex")).toBe("-1");
});

test("base rows compose handlers and preserve default opening and interactive children", () => {
  const open = vi.fn();
  const click = vi.fn();
  const cancel = vi.fn((event: { preventDefault: () => void }) =>
    event.preventDefault(),
  );
  render(
    <Table>
      <tbody>
        <TableRow onOpen={open} onClick={click}>
          <td>
            Normal <button>Action</button>
          </td>
        </TableRow>
        <TableRow onOpen={open} onClick={cancel}>
          <td>Consumed</td>
        </TableRow>
      </tbody>
    </Table>,
  );
  fireEvent.click(screen.getByText("Normal"));
  expect(open).toHaveBeenCalledTimes(1);
  expect(click).toHaveBeenCalledTimes(1);
  fireEvent.click(within(screen.getByText("Normal")).getByRole("button"));
  expect(open).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByText("Consumed"));
  expect(cancel).toHaveBeenCalledTimes(1);
  expect(open).toHaveBeenCalledTimes(1);
});
