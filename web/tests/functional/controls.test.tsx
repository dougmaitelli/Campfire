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
    if (!canOperate) {
      for (const action of ["Start", "Stop", "Restart"])
        expect(
          screen.queryByRole("button", { name: `${action} Test world` }),
        ).toBeNull();
      return;
    }
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

test.each(["install", "reinstall", "update"] as const)(
  "%s allows stop and kill while blocking other mutations",
  async (operation) => {
    const action = vi.fn();
    const user = userEvent.setup();
    render(
      <ServerLifecycleControls
        status={runtimeStatus({ state: "installing", operation, busy: true })}
        disabled={false}
        canReinstall
        onAction={action}
      />,
    );
    // jsdom does not implement the browser's popover API.
    const menu = screen.getByRole("menu", { hidden: true });
    menu.removeAttribute("popover");
    Object.assign(menu, { hidePopover: vi.fn() });
    for (const name of ["Start", "Restart"]) {
      expect(
        screen.getByRole<HTMLButtonElement>("button", { name }).disabled,
      ).toBe(true);
    }
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Stop" }).disabled,
    ).toBe(false);
    expect(
      screen.getByRole<HTMLButtonElement>("menuitem", { name: "Kill" })
        .disabled,
    ).toBe(false);
    expect(
      screen.getByRole<HTMLButtonElement>("menuitem", { name: "Reinstall" })
        .disabled,
    ).toBe(true);
    await user.click(screen.getByRole("button", { name: "Stop" }));
    await user.click(screen.getByRole("menuitem", { name: "Kill" }));
    expect(action.mock.calls).toEqual([["stop"], ["kill"]]);
  },
);

test("servers awaiting installation keep Start and Reinstall disabled", () => {
  render(
    <ServerLifecycleControls
      status={runtimeStatus({ state: "not_installed" })}
      disabled={false}
      canReinstall
      onAction={vi.fn()}
    />,
  );
  expect(
    screen.getByRole<HTMLButtonElement>("button", { name: "Start" }).disabled,
  ).toBe(true);
  expect(
    screen.getByRole<HTMLButtonElement>("menuitem", {
      name: "Reinstall",
      hidden: true,
    }).disabled,
  ).toBe(true);
});

test("server management actions remain available independently of power permission", async () => {
  const edit = vi.fn();
  render(
    <ServerLifecycleControls
      status={runtimeStatus()}
      disabled={false}
      canOperate={false}
      canReinstall={false}
      actions={[{ label: "Edit server", icon: "rename", onSelect: edit }]}
      onAction={vi.fn()}
    />,
  );
  for (const name of ["Start", "Stop", "Restart"])
    expect(screen.queryByRole("button", { name })).toBeNull();
  const menu = screen.getByRole("menu", { hidden: true });
  menu.removeAttribute("popover");
  Object.assign(menu, { hidePopover: vi.fn() });
  expect(screen.queryByRole("menuitem", { name: "Kill" })).toBeNull();
  await userEvent
    .setup()
    .click(screen.getByRole("menuitem", { name: "Edit server" }));
  expect(edit).toHaveBeenCalledOnce();
});

test("Delete remains the last server action after power operations", () => {
  render(
    <ServerLifecycleControls
      status={runtimeStatus()}
      disabled={false}
      canReinstall
      actions={[
        { label: "Delete", icon: "delete", tone: "danger", onSelect: vi.fn() },
        { label: "Edit server", icon: "rename", onSelect: vi.fn() },
      ]}
      onAction={vi.fn()}
    />,
  );
  expect(
    screen
      .getAllByRole("menuitem", { hidden: true })
      .map((item) => item.textContent),
  ).toEqual(["Edit server", "Kill", "Reinstall", "Delete"]);
});
