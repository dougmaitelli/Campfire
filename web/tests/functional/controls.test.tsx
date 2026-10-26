import { useState } from "react";
import { expect, test, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { RadioField } from "../../src/components/forms/RadioField";
import { ServerQuickControls } from "../../src/components/servers/ServerQuickControls";
import { ServerLifecycleControls } from "../../src/components/servers/ServerLifecycleControls";
import { runtimeStatus } from "../fixtures";

function InstallMode() {
  const [mode, setMode] = useState("keep");
  return (
    <>
      <RadioField
        name="install-mode"
        value="keep"
        checked={mode === "keep"}
        onChange={() => setMode("keep")}
        description="Preserve game files."
      >
        Keep existing files
      </RadioField>
      <RadioField
        name="install-mode"
        value="clean"
        checked={mode === "clean"}
        onChange={() => setMode("clean")}
      >
        Clean install
      </RadioField>
    </>
  );
}

test("radio fields retain native grouping, descriptions, and selection behavior", async () => {
  const user = userEvent.setup();
  render(<InstallMode />);
  const keep = screen.getByRole<HTMLInputElement>("radio", {
    name: /Keep existing files/,
  });
  const clean = screen.getByRole<HTMLInputElement>("radio", {
    name: "Clean install",
  });
  expect(keep.checked).toBe(true);
  expect(keep.name).toBe(clean.name);
  expect(keep.value).toBe("keep");
  expect(screen.getByText("Preserve game files.")).toBeDefined();
  await user.click(clean);
  expect(clean.checked).toBe(true);
  expect(keep.checked).toBe(false);
});

test("disabled radio fields cannot change selection", async () => {
  const change = vi.fn();
  render(
    <RadioField
      name="install-mode"
      value="keep"
      checked
      disabled
      onChange={change}
    >
      Keep files
    </RadioField>,
  );
  await userEvent.setup().click(screen.getByRole("radio"));
  expect(change).not.toHaveBeenCalled();
});

test.each([
  ["running", false, undefined, true, true, false, false],
  ["stopped", false, undefined, true, false, true, true],
  ["restarting", false, undefined, true, true, false, false],
  ["running", true, "restart", true, true, true, true],
  ["installing", true, "install", true, true, false, true],
  ["stopped", false, undefined, false, true, true, true],
  ["running", false, undefined, false, true, true, true],
  ["not_installed", false, undefined, true, true, true, true],
  ["unknown", false, undefined, true, true, true, true],
] as const)(
  "quick controls respect %s state and permissions",
  (
    state,
    busy,
    operation,
    canOperate,
    startDisabled,
    stopDisabled,
    restartDisabled,
  ) => {
    render(
      <ServerQuickControls
        id="server/one"
        name="Test world"
        status={runtimeStatus({
          state,
          busy,
          ...(operation ? { operation } : {}),
        })}
        disabled={false}
        canOperate={canOperate}
        onStatus={vi.fn()}
        onError={vi.fn()}
      />,
    );
    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: "Start Test world",
      }).disabled,
    ).toBe(startDisabled);
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Stop Test world" })
        .disabled,
    ).toBe(stopDisabled);
    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: "Restart Test world",
      }).disabled,
    ).toBe(restartDisabled);
  },
);

test.each(["install", "reinstall", "update"])(
  "%s allows stop and kill while blocking other mutations",
  async (operation) => {
    const action = vi.fn();
    const user = userEvent.setup();
    render(
      <ServerLifecycleControls
        status={runtimeStatus({ state: "installing", operation, busy: true })}
        disabled={false}
        canConfigure
        onAction={action}
      />,
    );
    for (const button of screen.getAllByRole<HTMLButtonElement>("button")) {
      const name = button.getAttribute("aria-label") ?? button.textContent;
      expect(button.disabled).toBe(!["Stop", "Kill"].includes(name ?? ""));
    }
    await user.click(screen.getByRole("button", { name: "Stop" }));
    await user.click(screen.getByRole("button", { name: "Kill" }));
    expect(action.mock.calls).toEqual([["stop"], ["kill"]]);
  },
);
