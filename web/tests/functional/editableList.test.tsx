import { useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { EditableList } from "../../src/components/forms/EditableList";

const initial = [
  { id: "a", name: "Alpha" },
  { id: "b", name: "Beta" },
];

test("existing editable lists keep remove and add controls without ordering controls", () => {
  const remove = vi.fn();
  render(
    <EditableList
      items={initial}
      rowLabel={(index) => initial[index]!.name}
      removeLabel={(index) => `Remove ${initial[index]!.name}`}
      addLabel="Add item"
      onAdd={() => {}}
      onRemove={remove}
    >
      {(item) => <input aria-label={item.name} defaultValue={item.name} />}
    </EditableList>,
  );
  expect(
    screen.queryByRole("button", { name: /Move .* (up|down)/ }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Remove Beta" }));
  expect(remove).toHaveBeenCalledWith(1);
  expect(screen.getByRole("button", { name: "Add item" })).toBeTruthy();
});

test("opt-in ordering preserves row identity and disables boundary moves", () => {
  function Editor() {
    const [items, setItems] = useState(initial);
    return (
      <EditableList
        items={items}
        itemKey={(item) => item.id}
        reorderable
        onMove={(from, to) =>
          setItems((current) => {
            const next = [...current];
            const [item] = next.splice(from, 1);
            if (item) next.splice(to, 0, item);
            return next;
          })
        }
        rowLabel={(index) => items[index]!.name}
        removeLabel={(index) => `Remove ${items[index]!.name}`}
        addLabel="Add item"
        onAdd={() => {}}
        onRemove={(index) =>
          setItems((current) =>
            current.filter((_, position) => position !== index),
          )
        }
      >
        {(item) => <input aria-label={item.name} defaultValue={item.name} />}
      </EditableList>
    );
  }
  render(<Editor />);
  const alpha = screen.getByRole("textbox", { name: "Alpha" });
  fireEvent.change(alpha, { target: { value: "Edited Alpha" } });
  expect(
    screen
      .getByRole("button", { name: "Move Alpha up" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "Move Beta down" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Move Alpha down" }));
  expect(
    screen
      .getAllByRole("textbox")
      .map((input) => input.getAttribute("aria-label")),
  ).toEqual(["Beta", "Alpha"]);
  expect(screen.getByRole("textbox", { name: "Alpha" })).toBe(alpha);
  expect((alpha as HTMLInputElement).value).toBe("Edited Alpha");
  expect(
    screen
      .getByRole("button", { name: "Move Alpha down" })
      .hasAttribute("disabled"),
  ).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Move Alpha up" }));
  expect(
    screen
      .getAllByRole("textbox")
      .map((input) => input.getAttribute("aria-label")),
  ).toEqual(["Alpha", "Beta"]);
  fireEvent.click(screen.getByRole("button", { name: "Remove Alpha" }));
  expect(screen.queryByRole("textbox", { name: "Alpha" })).toBeNull();
  expect(
    screen
      .getByRole("button", { name: "Move Beta up" })
      .hasAttribute("disabled"),
  ).toBe(true);
  expect(
    screen
      .getByRole("button", { name: "Move Beta down" })
      .hasAttribute("disabled"),
  ).toBe(true);
});
