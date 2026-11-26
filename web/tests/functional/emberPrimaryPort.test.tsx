import { useState, type ComponentProps } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, test, vi } from "vitest";
import { EmberPortFields } from "../../src/components/embers/EmberPortFields";
import {
  createEmberDraft,
  emptyEmber,
} from "../../src/components/embers/emberDraft";
import { NetworkProtocol } from "../../src/contracts/domains";

vi.mock("../../src/components/forms/Select", () => ({
  Select: ({
    onValueChange,
    ...props
  }: ComponentProps<"select"> & { onValueChange: (value: string) => void }) => (
    <select
      {...props}
      onChange={(event) => onValueChange(event.target.value)}
    />
  ),
}));

function Form({ legacy = false, portless = false }) {
  const [draft, setDraft] = useState(() =>
    createEmberDraft({
      ...emptyEmber(),
      schema: legacy ? 1 : 2,
      required_ports: portless
        ? []
        : [
            { container: 25575, protocol: NetworkProtocol.TCP },
            { container: 25565, protocol: NetworkProtocol.TCP },
          ],
    }),
  );
  return (
    <>
      <EmberPortFields draft={draft} onChange={setDraft} />
      <output data-testid="definition">
        {JSON.stringify(draft.definition)}
      </output>
    </>
  );
}
function definition() {
  return JSON.parse(screen.getByTestId("definition").textContent!);
}

test("older definitions default the first port on load, permit no primary, and do not infer after removal", async () => {
  render(<Form legacy />);
  const user = userEvent.setup();
  expect(definition().schema).toBe(2);
  expect(
    (
      screen.getByRole("combobox", {
        name: "Primary game port",
      }) as HTMLSelectElement
    ).value,
  ).toBe("0");
  const choose = async (name: string) => {
    await user.selectOptions(
      screen.getByRole("combobox", { name: "Primary game port" }),
      screen.getByRole("option", { name }),
    );
  };
  await choose("25565/tcp");
  expect(definition().schema).toBe(2);
  expect(
    definition().required_ports.map(
      (port: { primary: boolean }) => port.primary,
    ),
  ).toEqual([false, true]);
  await choose("25575/tcp");
  expect(
    definition().required_ports.map(
      (port: { primary: boolean }) => port.primary,
    ),
  ).toEqual([true, false]);
  await choose("No primary port");
  expect(
    definition().required_ports.every(
      (port: { primary: boolean }) => !port.primary,
    ),
  ).toBe(true);
  await choose("25565/tcp");
  await user.click(screen.getByRole("button", { name: "Remove port 2" }));
  expect(definition().required_ports).toHaveLength(1);
  expect(definition().required_ports[0].primary).toBe(false);
});

test("a portless older definition uses the current schema and no primary", async () => {
  render(<Form legacy portless />);
  const user = userEvent.setup();
  await user.selectOptions(
    screen.getByRole("combobox", { name: "Primary game port" }),
    "-1",
  );
  expect(definition().schema).toBe(2);
  expect(definition().required_ports).toEqual([]);
});
