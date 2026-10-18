import type { ComponentProps, ReactNode } from "react";
import { FormField } from "./FormField";
import { TextInput } from "./TextInput";

export function SearchField({
  label = "Search",
  ...props
}: Omit<ComponentProps<"input">, "type" | "children"> & { label?: ReactNode }) {
  return (
    <FormField label={label} className="search-field">
      <TextInput {...props} type="search" />
    </FormField>
  );
}
