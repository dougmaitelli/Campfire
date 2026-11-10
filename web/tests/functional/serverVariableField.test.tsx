import { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import type { EmberVariable } from "../../src/api/embers";
import { ServerVariableField } from "../../src/components/servers/ServerVariableField";
import { ServerReview } from "../../src/components/servers/ServerReview";
import { serverConfigurationChanges } from "../../src/utils/serverConfigurationChanges";

const variable: EmberVariable = {
  name: "ENABLE_CROSSPLAY",
  default: "0",
  required: true,
  editable: true,
  validation: { type: "boolean" },
};

test("boolean variables preserve 0/1 strings and allow required false values", async () => {
  const changed = vi.fn();
  function Form() {
    const [value, setValue] = useState(variable.default);
    return (
      <form aria-label="Settings">
        <ServerVariableField
          variable={variable}
          value={value}
          disabled={false}
          onChange={(next) => {
            changed(next);
            setValue(next);
          }}
        />
      </form>
    );
  }
  render(<Form />);
  const toggle = screen.getByRole("switch", {
    name: "ENABLE_CROSSPLAY",
  }) as HTMLInputElement;
  expect(toggle.checked).toBe(false);
  expect((screen.getByRole("form") as HTMLFormElement).checkValidity()).toBe(
    true,
  );
  const user = userEvent.setup();
  await user.click(toggle);
  expect(changed).toHaveBeenLastCalledWith("1");
  expect(toggle.checked).toBe(true);
  await user.keyboard(" ");
  expect(changed).toHaveBeenLastCalledWith("0");
  expect(toggle.checked).toBe(false);
  expect(toggle.checkValidity()).toBe(true);
});

test.each([
  { editable: false, disabled: false },
  { editable: true, disabled: true },
])(
  "boolean editing respects restrictions %o",
  async ({ editable, disabled }) => {
    const changed = vi.fn();
    render(
      <ServerVariableField
        variable={{ ...variable, editable }}
        value="1"
        disabled={disabled}
        onChange={changed}
      />,
    );
    const toggle = screen.getByRole("switch") as HTMLInputElement;
    expect(toggle.checked).toBe(true);
    expect(toggle.disabled).toBe(true);
    await userEvent.setup().click(toggle);
    expect(changed).not.toHaveBeenCalled();
  },
);

test("untyped 0/1 variables remain text inputs", () => {
  render(
    <ServerVariableField
      variable={{
        name: "TEXT_SETTING",
        default: "1",
        required: false,
        editable: true,
      }}
      value="1"
      disabled={false}
      onChange={vi.fn()}
    />,
  );
  expect(screen.queryByRole("switch")).toBeNull();
  expect(
    (screen.getByRole("textbox", { name: "TEXT_SETTING" }) as HTMLInputElement)
      .value,
  ).toBe("1");
});

test("change descriptions preserve control names and disappear when a value is reverted", () => {
  const props = { variable, value: "1", disabled: false, onChange: vi.fn() };
  const view = render(<ServerVariableField {...props} change="added" />);
  const toggle = screen.getByRole("switch", {
    name: "ENABLE_CROSSPLAY",
  });
  expect(screen.getByLabelText("ENABLE_CROSSPLAY", { exact: true })).toBe(
    toggle,
  );
  expect(toggle.closest(".form-field")?.getAttribute("data-change")).toBe(
    "added",
  );
  expect(
    document.getElementById(toggle.getAttribute("aria-describedby")!)
      ?.textContent,
  ).toBe("Added");
  view.rerender(<ServerVariableField {...props} />);
  expect(toggle.closest(".form-field")?.hasAttribute("data-change")).toBe(
    false,
  );
  expect(toggle.hasAttribute("aria-describedby")).toBe(false);
});

test("update reviews mark saved-value changes, removed variables, and reset startup overrides", () => {
  const value = {
    name: "Test",
    description: "",
    spark_id: "spark",
    ember_id: "ember",
    image: "new",
    cpu_limit: 1,
    memory_mb: 1024,
    disk_mb: 1024,
    ports: [],
    variables: { ADDED: "" },
  };
  const before = {
    ...value,
    image: "old",
    startup_override: "custom",
    variables: { REMOVED: "old value" },
  };
  render(
    <ServerReview
      value={value}
      changes={serverConfigurationChanges(before, value)}
      sparkName="Spark"
      emberName="Ember"
      notice=""
    />,
  );
  expect(
    screen
      .getByText("ADDED", { exact: true })
      .closest("div")
      ?.getAttribute("data-change"),
  ).toBe("added");
  expect(
    screen
      .getByText("REMOVED", { exact: true })
      .closest("div")
      ?.getAttribute("data-change"),
  ).toBe("removed");
  expect(
    screen
      .getByText("Use Ember default")
      .closest("div")
      ?.getAttribute("data-change"),
  ).toBe("changed");
  expect(
    screen
      .getByText("new", { exact: true })
      .closest("div")
      ?.getAttribute("data-change"),
  ).toBe("changed");
});

test("read-only reviews format only declared booleans and preserve empty values", () => {
  render(
    <ServerReview
      value={{
        name: "Test",
        description: "",
        spark_id: "spark",
        ember_id: "ember",
        image: "image",
        cpu_limit: 1,
        memory_mb: 1024,
        disk_mb: 1024,
        ports: [],
        variables: { FLAG_ON: "1", FLAG_OFF: "0", TEXT: "1", EMPTY: "" },
      }}
      sparkName="Spark"
      emberName="Ember"
      variableTypes={{
        FLAG_ON: "boolean",
        FLAG_OFF: "boolean",
        EMPTY: "boolean",
      }}
      notice=""
    />,
  );
  for (const [name, checked] of [
    ["FLAG_ON", true],
    ["FLAG_OFF", false],
  ] as const) {
    const toggle = screen.getByRole<HTMLInputElement>("switch", { name });
    expect(toggle.checked).toBe(checked);
    expect(toggle.disabled).toBe(true);
  }
  for (const [name, value] of [
    ["TEXT", "1"],
    ["EMPTY", "(empty)"],
  ]) {
    expect(
      screen.getByText(name!, { exact: true }).closest("div")?.textContent,
    ).toBe(`${name}${value}`);
  }
});
