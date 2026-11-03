import { beforeAll, beforeEach, afterAll, expect, test, vi } from "vitest";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { EmberPreview } from "../../src/api/embers";
import { importEmber, previewEmberDefinition } from "../../src/api/embers";
import { ImportEmberDrawer } from "../../src/drawers/ImportEmberDrawer";
import { completeEmber } from "../fixtures/ember";

vi.mock("../../src/api/embers", () => ({
  importEmber: vi.fn(),
  deleteEmber: vi.fn(),
  previewEmberURL: vi.fn(),
  previewEmberDefinition: vi.fn(),
}));

const dialogMethods = Object.getOwnPropertyDescriptors(
  HTMLDialogElement.prototype,
);
beforeAll(() => {
  Object.defineProperties(HTMLDialogElement.prototype, {
    showModal: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.setAttribute("open", "");
      },
    },
    close: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.removeAttribute("open");
      },
    },
  });
});
afterAll(() => {
  for (const name of ["showModal", "close"]) {
    const original = dialogMethods[name];
    if (original)
      Object.defineProperty(HTMLDialogElement.prototype, name, original);
    else Reflect.deleteProperty(HTMLDialogElement.prototype, name);
  }
});

beforeEach(() => {
  vi.mocked(previewEmberDefinition).mockImplementation(async (definition) => {
    try {
      return {
        format: "ember",
        definition,
        ember: JSON.parse(definition),
        warnings: [],
        errors: [],
      };
    } catch {
      return {
        format: "unknown",
        definition,
        ember: null,
        warnings: [],
        errors: [{ field: "definition", message: "Invalid JSON" }],
      };
    }
  });
  vi.mocked(importEmber).mockImplementation(async (definition) =>
    JSON.parse(definition),
  );
});

function editor() {
  const saved = vi.fn();
  render(
    <ImportEmberDrawer
      ember={completeEmber()}
      onClose={vi.fn()}
      onImported={saved}
    />,
  );
  return saved;
}

test("actionable error rules can be authored without IDs and survive JSON switching", async () => {
  const user = userEvent.setup();
  editor();
  await user.click(screen.getByRole("button", { name: "Runtime" }));
  await user.click(screen.getByRole("button", { name: "Add error rule" }));
  const rule = within(screen.getByRole("group", { name: "Error rule 1" }));
  fireEvent.change(rule.getByLabelText("Issue title"), {
    target: { value: "EULA requires acceptance" },
  });
  fireEvent.change(rule.getByLabelText("What the user should do"), {
    target: { value: "Review eula.txt" },
  });
  fireEvent.change(rule.getByLabelText("Message or pattern"), {
    target: { value: "agree to the EULA" },
  });
  await user.click(rule.getByLabelText("Ignore letter case"));
  await user.click(rule.getByRole("button", { name: "Add condition" }));
  const other = within(rule.getByRole("group", { name: "Error condition 2" }));
  fireEvent.change(other.getByLabelText("Message or pattern"), {
    target: { value: "^Exact error$" },
  });
  await user.click(other.getByLabelText("Use a regular expression"));
  expect(
    other.getByLabelText<HTMLInputElement>("Ignore letter case").checked,
  ).toBe(false);
  expect(screen.queryByLabelText("Ignore console color codes")).toBeNull();
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  const definition = JSON.parse(
    screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Definition" })
      .value,
  );
  expect(definition.log_alerts).toEqual([
    {
      title: "EULA requires acceptance",
      message: "Review eula.txt",
      severity: "error",
      any: [
        { value: "agree to the EULA", ignore_case: true },
        { value: "^Exact error$", regex: true },
      ],
    },
  ]);
  expect(definition.log_alerts[0]).not.toHaveProperty("id");
  await user.click(screen.getByRole("tab", { name: "Visual editor" }));
  expect(screen.getByLabelText<HTMLInputElement>("Issue title").value).toBe(
    "EULA requires acceptance",
  );
});

test("visual edits save without losing advanced options from JSON", async () => {
  const user = userEvent.setup();
  editor();
  const name = screen.getByRole("textbox", { name: "Name" });
  fireEvent.change(name, { target: { value: "Edited game" } });
  await user.click(screen.getByRole("button", { name: "Review" }));
  const save = screen.getByRole<HTMLButtonElement>("button", {
    name: "Save Ember",
  });
  await waitFor(() => expect(save.disabled).toBe(false));
  expect(screen.getByText("Definition is valid")).toBeDefined();
  expect(screen.queryByText(/ready to import/)).toBeNull();
  await user.click(save);
  await waitFor(() => expect(importEmber).toHaveBeenCalled());
  const submitted = JSON.parse(vi.mocked(importEmber).mock.calls.at(-1)![0]);
  expect(submitted).toEqual({ ...completeEmber(), name: "Edited game" });
});

test("native import metadata without warnings can be reviewed", async () => {
  const user = userEvent.setup();
  const ember = completeEmber();
  ember.origin!.warnings = null;
  render(
    <ImportEmberDrawer ember={ember} onClose={vi.fn()} onImported={vi.fn()} />,
  );
  await user.click(screen.getByRole("button", { name: "Review" }));
  await user.click(screen.getByText("Import metadata"));
  expect(screen.getByText("Originally declared update URL")).toBeDefined();
  await waitFor(() =>
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Save Ember" })
        .disabled,
    ).toBe(false),
  );
});

test("pasted JSON updates the visual fields and survives switching back", async () => {
  const user = userEvent.setup();
  editor();
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  const definition = {
    ...completeEmber(),
    name: "Pasted game",
    description: "Kept from pasted JSON",
  };
  fireEvent.change(screen.getByRole("textbox", { name: "Definition" }), {
    target: { value: JSON.stringify(definition) },
  });
  const visual = screen.getByRole<HTMLButtonElement>("tab", {
    name: "Visual editor",
  });
  await waitFor(() => expect(visual.disabled).toBe(false));
  await user.click(visual);
  expect(
    screen.getByRole<HTMLInputElement>("textbox", { name: "Name" }).value,
  ).toBe("Pasted game");
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  expect(
    JSON.parse(
      screen.getByRole<HTMLTextAreaElement>("textbox", {
        name: "Definition",
      }).value,
    ),
  ).toEqual(definition);
});

test("invalid pasted JSON stays intact and cannot be overwritten by the visual editor", async () => {
  const user = userEvent.setup();
  editor();
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  const definition = screen.getByRole<HTMLTextAreaElement>("textbox", {
    name: "Definition",
  });
  fireEvent.change(definition, { target: { value: '{"schema":1,' } });
  const visual = screen.getByRole<HTMLButtonElement>("tab", {
    name: "Visual editor",
  });
  await waitFor(() => expect(visual.disabled).toBe(false));
  await user.click(visual);
  expect(
    screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Definition",
    }).value,
  ).toBe('{"schema":1,');
  expect(
    screen
      .getAllByRole("alert")
      .some((item) =>
        item.textContent?.includes("Your JSON or YAML has been kept"),
      ),
  ).toBe(true);
});

test("duplicate image labels block JSON switching until the row is corrected", async () => {
  const user = userEvent.setup();
  editor();
  await user.click(screen.getByRole("button", { name: "Runtime" }));
  await user.click(screen.getByRole("button", { name: "Add image" }));
  const row = screen.getByRole("group", { name: "image 3" });
  fireEvent.change(within(row).getByLabelText("Image label"), {
    target: { value: "Alpine" },
  });
  fireEvent.change(within(row).getByLabelText("Image address"), {
    target: { value: "alpine:edge" },
  });
  expect(
    screen.getByRole<HTMLButtonElement>("tab", { name: "JSON / YAML" })
      .disabled,
  ).toBe(true);
  await user.click(screen.getByRole("tab", { name: "Visual editor" }));
  expect(
    within(row).getByLabelText<HTMLInputElement>("Image label").value,
  ).toBe("Alpine");
  fireEvent.change(within(row).getByLabelText("Image label"), {
    target: { value: "Edge" },
  });
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  const definition = JSON.parse(
    screen.getByRole<HTMLTextAreaElement>("textbox", {
      name: "Definition",
    }).value,
  );
  expect(definition.images).toEqual({
    ...completeEmber().images,
    Edge: "alpine:edge",
  });
});

test("the selected step and entered values survive switching editor tabs", async () => {
  const user = userEvent.setup();
  editor();
  await user.click(screen.getByRole("button", { name: "Runtime" }));
  fireEvent.change(screen.getByLabelText("Server user"), {
    target: { value: "2000:2000" },
  });
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  await user.click(screen.getByRole("tab", { name: "Visual editor" }));
  expect(screen.getByRole("heading", { level: 3 }).textContent).toBe("Runtime");
  expect(screen.getByLabelText<HTMLInputElement>("Server user").value).toBe(
    "2000:2000",
  );
});

test("the pattern is part of the optional validation rules and is removed when disabled", async () => {
  const user = userEvent.setup();
  editor();
  await user.click(screen.getByRole("button", { name: "Variables" }));
  const variable = within(screen.getByRole("group", { name: "LABEL" }));
  expect(variable.queryByLabelText("Validation pattern (RE2)")).toBeNull();
  await user.click(variable.getByLabelText("Add validation rules"));
  fireEvent.change(variable.getByLabelText("Validation pattern (RE2)"), {
    target: { value: "^Label" },
  });
  await user.click(variable.getByLabelText("Add validation rules"));
  expect(variable.queryByLabelText("Validation pattern (RE2)")).toBeNull();
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  const definition = JSON.parse(
    screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Definition" })
      .value,
  );
  expect(definition.variables[2].pattern).toBeUndefined();
  expect(definition.variables[2].validation).toBeUndefined();
});

test("Read-only is the inverse of the Ember editable flag", async () => {
  const user = userEvent.setup();
  editor();
  await user.click(screen.getByRole("button", { name: "Variables" }));
  const variable = within(screen.getByRole("group", { name: "LABEL" }));
  const readOnly = variable.getByRole<HTMLInputElement>("checkbox", {
    name: "Read-only",
  });
  expect(readOnly.checked).toBe(false);
  await user.click(readOnly);
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  const definition = JSON.parse(
    screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Definition" })
      .value,
  );
  expect(definition.variables[2].editable).toBe(false);
  await user.click(screen.getByRole("tab", { name: "Visual editor" }));
  expect(
    within(
      screen.getByRole("group", { name: "LABEL" }),
    ).getByRole<HTMLInputElement>("checkbox", { name: "Read-only" }).checked,
  ).toBe(true);
});

test("the JSON tab shows the generated Ember after viewing a converted definition", async () => {
  const user = userEvent.setup();
  render(
    <ImportEmberDrawer
      initialPreview={{
        format: "PLCN_v3",
        definition: "meta:\n  version: PLCN_v3\n",
        ember: completeEmber(),
        errors: [],
        warnings: [],
      }}
      onClose={vi.fn()}
      onImported={vi.fn()}
    />,
  );
  await user.click(screen.getByRole("tab", { name: "Visual editor" }));
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  const definition = screen.getByRole<HTMLTextAreaElement>("textbox", {
    name: "Definition",
  });
  expect(JSON.parse(definition.value)).toEqual(completeEmber());
});

test("editing the update URL while an image is incomplete still updates the download source", async () => {
  const user = userEvent.setup();
  const original = completeEmber();
  render(
    <ImportEmberDrawer
      ember={original}
      initialSourceURL={original.updateUrl}
      onClose={vi.fn()}
      onImported={vi.fn()}
    />,
  );
  await user.click(screen.getByRole("button", { name: "Runtime" }));
  await user.click(screen.getByRole("button", { name: "Add image" }));
  await user.click(screen.getByRole("button", { name: "Basics" }));
  fireEvent.change(screen.getByLabelText("Update URL"), {
    target: { value: "https://example.com/new.yaml" },
  });
  await user.click(screen.getByRole("button", { name: "Runtime" }));
  const image = within(screen.getByRole("group", { name: "image 3" }));
  fireEvent.change(image.getByLabelText("Image label"), {
    target: { value: "Edge" },
  });
  fireEvent.change(image.getByLabelText("Image address"), {
    target: { value: "alpine:edge" },
  });
  await user.click(screen.getByRole("button", { name: "Review" }));
  const save = screen.getByRole<HTMLButtonElement>("button", {
    name: "Save Ember",
  });
  await waitFor(() => expect(save.disabled).toBe(false));
  await user.click(save);
  await waitFor(() => expect(importEmber).toHaveBeenCalled());
  const submitted = vi.mocked(importEmber).mock.calls.at(-1)!;
  expect(submitted[2]).toBe("https://example.com/new.yaml");
  expect(JSON.parse(submitted[0]).updateUrl).toBe(submitted[2]);
});

test("setup acknowledgements remain required after converting an egg and changing visual fields", async () => {
  const user = userEvent.setup();
  const definition = JSON.stringify(completeEmber());
  const preview: EmberPreview = {
    format: "PLCN_v3",
    definition,
    ember: completeEmber(),
    errors: [],
    warnings: [{ field: "installer", message: "Review the install script" }],
  };
  render(
    <ImportEmberDrawer
      initialPreview={preview}
      onClose={vi.fn()}
      onImported={vi.fn()}
    />,
  );
  await user.click(screen.getByRole("tab", { name: "Visual editor" }));
  fireEvent.change(screen.getByLabelText("Name", { exact: true }), {
    target: { value: "Converted game" },
  });
  await user.click(screen.getByRole("tab", { name: "JSON / YAML" }));
  const rawDefinition = screen.getByRole<HTMLTextAreaElement>("textbox", {
    name: "Definition",
  });
  fireEvent.change(rawDefinition, {
    target: { value: `${rawDefinition.value}\n` },
  });
  const save = screen.getByRole<HTMLButtonElement>("button", {
    name: "Import Ember",
  });
  await waitFor(() =>
    expect(screen.getByText("Review the install script")).toBeDefined(),
  );
  expect(save.disabled).toBe(true);
  await user.click(
    screen.getByRole("checkbox", {
      name: "I understand these setup requirements.",
    }),
  );
  await waitFor(() => expect(save.disabled).toBe(false));
  const unrelated = completeEmber();
  delete unrelated.origin;
  fireEvent.change(rawDefinition, {
    target: { value: JSON.stringify(unrelated) },
  });
  await waitFor(() =>
    expect(screen.queryByText("Review the install script")).toBeNull(),
  );
  await waitFor(() => expect(save.disabled).toBe(false));
});
