import { fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { expect, test } from "vitest";
import {
  PasswordInput,
  passwordPolicy,
} from "../../src/components/forms/PasswordInput";
import { FormField } from "../../src/components/forms/FormField";

test("password rules update while typing and preserve the field label and help", () => {
  function Example() {
    const [value, setValue] = useState("");
    return (
      <FormField label="New password" description="Choose a unique password.">
        <PasswordInput
          value={value}
          onChange={(event) => setValue(event.target.value)}
        />
      </FormField>
    );
  }
  render(<Example />);
  const input = screen.getByLabelText("New password");
  expect(
    screen.queryByRole("list", { name: "Password requirements" }),
  ).toBeNull();
  fireEvent.change(input, { target: { value: "short" } });
  const rules = screen.getByRole("list", { name: "Password requirements" });
  expect(within(rules).getAllByText("Satisfied:")).toHaveLength(1);
  fireEvent.change(input, { target: { value: "A long passphrase 1!" } });
  expect(within(rules).getAllByText("Satisfied:")).toHaveLength(5);
  expect(rules.querySelectorAll(".password-rule-met")).toHaveLength(5);
  expect(screen.queryByText(/bytes/)).toBeNull();
  expect(input.getAttribute("aria-describedby")).toContain(rules.id);
  expect(input.getAttribute("aria-describedby")).toContain(
    screen.getByText("Choose a unique password.").id,
  );
  fireEvent.change(input, { target: { value: "" } });
  expect(
    screen.queryByRole("list", { name: "Password requirements" }),
  ).toBeNull();
});

test("password policy supports Unicode and requires every configured character category", () => {
  expect(passwordPolicy("😀".repeat(14)).minimum).toBe(false);
  expect(passwordPolicy("😀".repeat(15)).minimum).toBe(true);
  expect(
    Object.values(passwordPolicy("Äé١!" + "😀".repeat(11))).every(Boolean),
  ).toBe(true);
  expect(passwordPolicy("long lowercase password 1!").uppercase).toBe(false);
  expect(passwordPolicy("LONG UPPERCASE PASSWORD 1!").lowercase).toBe(false);
  expect(passwordPolicy("Long password without digit!").number).toBe(false);
  expect(passwordPolicy("Long password without symbol 1").symbol).toBe(false);
});
