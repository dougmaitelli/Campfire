import { StrictMode, useState } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { BaseDialog } from "../../src/components/layout/BaseDialog";

beforeEach(() => {
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute("open", "");
  };
  HTMLDialogElement.prototype.close = function () {
    this.removeAttribute("open");
    this.dispatchEvent(new Event("close", { bubbles: true }));
  };
});

test("nested portaled dialogs isolate Escape and block dismissal while busy", () => {
  const parentClosed = vi.fn();
  const childClosed = vi.fn();
  function Dialogs({ busy }: { busy: boolean }) {
    const [child, setChild] = useState(true);
    return (
      <BaseDialog aria-label="Parent" busy={false} onClose={parentClosed}>
        <input aria-label="Draft" defaultValue="Keep this" />
        {child && (
          <BaseDialog
            portal
            aria-label="Child"
            busy={busy}
            dismissOnBackdrop
            onClose={() => {
              childClosed();
              setChild(false);
            }}
          />
        )}
      </BaseDialog>
    );
  }
  const view = render(<Dialogs busy />);
  const child = screen.getByRole("dialog", { name: "Child" });
  fireEvent(child, new Event("cancel", { bubbles: true, cancelable: true }));
  fireEvent.click(child);
  expect(childClosed).not.toHaveBeenCalled();
  expect(parentClosed).not.toHaveBeenCalled();
  view.rerender(<Dialogs busy={false} />);
  fireEvent(child, new Event("cancel", { bubbles: true, cancelable: true }));
  expect(childClosed).toHaveBeenCalledOnce();
  expect(parentClosed).not.toHaveBeenCalled();
  expect((screen.getByLabelText("Draft") as HTMLInputElement).value).toBe(
    "Keep this",
  );
});

test("opening selects the configured input without treating cleanup as dismissal", () => {
  const closed = vi.fn();
  const view = render(
    <StrictMode>
      <BaseDialog
        busy={false}
        onClose={closed}
        initialFocus="#selected"
        selectInitialText
      >
        <input id="selected" aria-label="Selected" defaultValue="Rename me" />
      </BaseDialog>
    </StrictMode>,
  );
  const input = screen.getByLabelText("Selected") as HTMLInputElement;
  expect(document.activeElement).toBe(input);
  expect([input.selectionStart, input.selectionEnd]).toEqual([0, 9]);
  expect(closed).not.toHaveBeenCalled();
  view.unmount();
  expect(closed).not.toHaveBeenCalled();
});

test("controlled dialogs reopen without losing their contents", () => {
  const closed = vi.fn();
  const content = <input aria-label="Draft" defaultValue="Original" />;
  const view = render(
    <BaseDialog open busy={false} onClose={closed}>
      {content}
    </BaseDialog>,
  );
  fireEvent.change(screen.getByLabelText("Draft"), {
    target: { value: "Edited" },
  });
  view.rerender(
    <BaseDialog open={false} busy={false} onClose={closed}>
      {content}
    </BaseDialog>,
  );
  expect(screen.queryByRole("dialog")).toBeNull();
  view.rerender(
    <BaseDialog open busy={false} onClose={closed}>
      {content}
    </BaseDialog>,
  );
  expect((screen.getByLabelText("Draft") as HTMLInputElement).value).toBe(
    "Edited",
  );
  expect(closed).not.toHaveBeenCalled();
});
