import { useState } from "react";
import { expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Wizard } from "../../src/components/wizard/Wizard";

function TestWizard({ finish }: { finish: () => Promise<void> }) {
  const [name, setName] = useState("");
  const [cpu, setCPU] = useState("");
  const [passwordRequired, setPasswordRequired] = useState(false);
  return (
    <Wizard
      busy={false}
      error={null}
      finishLabel="Save"
      onFinish={finish}
      steps={[
        {
          id: "general",
          title: "General",
          validate: () => (name.trim() ? null : "Enter a name"),
          content: (
            <>
              <input
                aria-label="Name"
                required
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
              <label>
                <input
                  type="checkbox"
                  checked={passwordRequired}
                  onChange={(event) =>
                    setPasswordRequired(event.target.checked)
                  }
                />
                Require password
              </label>
              {passwordRequired && <input aria-label="Password" required />}
            </>
          ),
        },
        {
          id: "resources",
          title: "Resources",
          content: (
            <input
              aria-label="CPU"
              type="number"
              min={1}
              max={8}
              required
              value={cpu}
              onChange={(event) => setCPU(event.target.value)}
            />
          ),
        },
        { id: "review", title: "Review", content: <p>Ready to save</p> },
      ]}
    >
      {(content, actions) => (
        <>
          {content}
          {actions}
        </>
      )}
    </Wizard>
  );
}

test("wizard disables Next for empty, whitespace, and invalid numeric fields across steps", async () => {
  const user = userEvent.setup();
  const finish = vi.fn(async () => {});
  render(<TestWizard finish={finish} />);
  const next = screen.getByRole<HTMLButtonElement>("button", {
    name: "Next: Resources",
  });
  expect(next.disabled).toBe(true);
  await user.type(screen.getByLabelText("Name"), "   ");
  expect(next.disabled).toBe(true);
  await user.clear(screen.getByLabelText("Name"));
  await user.type(screen.getByLabelText("Name"), "Game server");
  expect(next.disabled).toBe(false);
  await user.click(next);
  const review = screen.getByRole<HTMLButtonElement>("button", {
    name: "Next: Review",
  });
  await waitFor(() => expect(review.disabled).toBe(true));
  await user.type(screen.getByLabelText("CPU"), "0");
  expect(review.disabled).toBe(true);
  await user.clear(screen.getByLabelText("CPU"));
  await user.type(screen.getByLabelText("CPU"), "2");
  await waitFor(() => expect(review.disabled).toBe(false));
  await user.click(review);
  const save = screen.getByRole<HTMLButtonElement>("button", { name: "Save" });
  await waitFor(() => expect(save.disabled).toBe(false));
  await user.click(save);
  expect(finish).toHaveBeenCalledOnce();
});

test("wizard rechecks validity when required fields are added and removed", async () => {
  const user = userEvent.setup();
  render(<TestWizard finish={vi.fn(async () => {})} />);
  await user.type(screen.getByLabelText("Name"), "Game server");
  const next = screen.getByRole<HTMLButtonElement>("button", {
    name: "Next: Resources",
  });
  expect(next.disabled).toBe(false);
  await user.click(screen.getByRole("checkbox", { name: "Require password" }));
  await waitFor(() => expect(next.disabled).toBe(true));
  await user.type(screen.getByLabelText("Password"), "secret");
  await waitFor(() => expect(next.disabled).toBe(false));
  await user.clear(screen.getByLabelText("Password"));
  await waitFor(() => expect(next.disabled).toBe(true));
  await user.click(screen.getByRole("checkbox", { name: "Require password" }));
  await waitFor(() => expect(next.disabled).toBe(false));
});
